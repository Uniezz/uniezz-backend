package study

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestListProfessors(t *testing.T) {
	e := newTestEnv(t)

	t.Run("lists active professors of a university by name, across pages", func(t *testing.T) {
		viewer := e.student(t)
		externalID := uuid.New()
		var id uuid.UUID
		err := e.pool.QueryRow(context.Background(), `
			INSERT INTO professors (external_id, radon_id, name, university_id)
			VALUES ($1, 'R-test', $2, 'wsei') RETURNING id`, externalID, "Test "+externalID.String()).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM professors WHERE id = $1", id) })

		found := false
		cursor := ""
		for range 500 {
			page := decode[itemPage[Professor]](t, e.api.Get("/study/professors?universityId=wsei&limit=50"+cursor, viewer.bearer), http.StatusOK)
			for _, p := range page.Items {
				if p.UniversityID != "wsei" {
					t.Fatalf("professor of another university: %#v", p)
				}
				found = found || p.ID == id
			}
			if page.NextCursor == nil {
				break
			}
			cursor = "&cursor=" + *page.NextCursor
		}
		if !found {
			t.Fatal("the professor is not listed")
		}
	})

	professor := func(t *testing.T, name, universityID string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := e.pool.QueryRow(context.Background(), `
			INSERT INTO professors (external_id, radon_id, name, university_id)
			VALUES ($1, 'R-test', $2, $3) RETURNING id`, uuid.New(), name, universityID).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM professors WHERE id = $1", id) })
		return id
	}

	t.Run("q matches the name ignoring case and accents, across pages", func(t *testing.T) {
		viewer := e.student(t)
		marker := uuid.NewString()[:8]
		want := map[uuid.UUID]bool{}
		for _, name := range []string{"Nowak " + marker, "Żak " + marker, "Ćwik " + marker} {
			want[professor(t, name, "wsei")] = true
		}
		professor(t, "Kowalski "+marker, "umcs")

		seen := map[uuid.UUID]bool{}
		cursor := ""
		for range 3 {
			page := decode[itemPage[Professor]](t, e.api.Get("/study/professors?universityId=wsei&limit=2&q="+marker+cursor, viewer.bearer), http.StatusOK)
			for _, p := range page.Items {
				if seen[p.ID] || !want[p.ID] {
					t.Fatalf("unexpected or repeated professor %#v", p)
				}
				seen[p.ID] = true
			}
			if page.NextCursor == nil {
				break
			}
			cursor = "&cursor=" + *page.NextCursor
		}
		if len(seen) != len(want) {
			t.Fatalf("saw %d of %d professors", len(seen), len(want))
		}

		page := decode[itemPage[Professor]](t, e.api.Get("/study/professors?q="+url.QueryEscape("ZAK "+marker), viewer.bearer), http.StatusOK)
		if len(page.Items) != 1 {
			t.Fatalf("professors = %#v", page.Items)
		}
	})

	t.Run("a q word shorter than 3 characters is invalid input", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/professors?q=ab", viewer.bearer), http.StatusBadRequest, "invalid_input")
	})

	t.Run("an unknown university is invalid input", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/professors?universityId=nowhere", viewer.bearer), http.StatusBadRequest, "invalid_input")
	})
}
