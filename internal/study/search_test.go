package study

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func pageIDs[T any](t *testing.T, e *testEnv, path string, viewer student, idOf func(T) uuid.UUID) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	cursor := ""
	for range 50 {
		page := decode[itemPage[T]](t, e.api.Get(path+"&limit=1"+cursor, viewer.bearer), http.StatusOK)
		for _, item := range page.Items {
			ids = append(ids, idOf(item))
		}
		if page.NextCursor == nil {
			return ids
		}
		cursor = "&cursor=" + url.QueryEscape(*page.NextCursor)
	}
	t.Fatal("too many pages")
	return nil
}

func TestSearchProfessors(t *testing.T) {
	e := newTestEnv(t)
	professorID := func(p Professor) uuid.UUID { return p.ID }

	insert := func(t *testing.T, name, university string) uuid.UUID {
		t.Helper()
		var id uuid.UUID
		err := e.pool.QueryRow(context.Background(), `
			INSERT INTO professors (external_id, radon_id, name, university_id)
			VALUES ($1, 'R-test', $2, $3) RETURNING id`, uuid.New(), name, university).Scan(&id)
		if err != nil {
			t.Fatalf("insert professor: %v", err)
		}
		t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM professors WHERE id = $1", id) })
		return id
	}

	t.Run("matches part of the name regardless of case and accents, page by page", func(t *testing.T) {
		viewer := e.student(t)
		marker := uuid.NewString()[:8]
		first := insert(t, "Anna Nowák "+marker, "wsei")
		second := insert(t, "Jan NOWAK "+marker, "wsei")
		insert(t, "Ewa Kowalska "+marker, "wsei")

		got := pageIDs(t, e, "/study/professors?q=nowak+"+marker, viewer, professorID)
		if len(got) != 2 || got[0] != first || got[1] != second {
			t.Fatalf("ids = %v, want [%s %s]", got, first, second)
		}
	})

	t.Run("search combines with the university filter", func(t *testing.T) {
		viewer := e.student(t)
		marker := uuid.NewString()[:8]
		want := insert(t, "Search "+marker, "wsei")
		insert(t, "Search "+marker, "umcs")

		got := pageIDs(t, e, "/study/professors?universityId=wsei&q="+marker, viewer, professorID)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("ids = %v, want [%s]", got, want)
		}
	})

	t.Run("a word shorter than 3 characters is invalid input", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/professors?q=ab", viewer.bearer), http.StatusBadRequest, "invalid_input")
	})
}

func TestSearchCourses(t *testing.T) {
	e := newTestEnv(t)
	courseID := func(c Course) uuid.UUID { return c.ID }

	t.Run("matches the name or the subject, newest first, page by page", func(t *testing.T) {
		author := e.student(t)
		marker := uuid.NewString()[:8]
		byName := e.course(t, author, map[string]any{"name": "Algebra " + marker})
		bySubject := e.course(t, author, map[string]any{"subject": "Topology " + marker})
		e.course(t, author, nil)

		got := pageIDs(t, e, "/study/courses?q="+marker, author, courseID)
		if len(got) != 2 || got[0] != bySubject.ID || got[1] != byName.ID {
			t.Fatalf("ids = %v, want [%s %s]", got, bySubject.ID, byName.ID)
		}
	})

	t.Run("every word must match and search combines with the semester filter", func(t *testing.T) {
		author := e.student(t)
		marker := uuid.NewString()[:8]
		want := e.course(t, author, map[string]any{"name": "Linear algebra " + marker, "semester": 3})
		e.course(t, author, map[string]any{"name": "Linear algebra " + marker, "semester": 4})
		e.course(t, author, map[string]any{"name": "Linear logic " + marker, "semester": 3})

		got := pageIDs(t, e, "/study/courses?semester=3&q=ALGEBRA+"+marker, author, courseID)
		if len(got) != 1 || got[0] != want.ID {
			t.Fatalf("ids = %v, want [%s]", got, want.ID)
		}
	})

	t.Run("a word shorter than 3 characters is invalid input", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/courses?q=ab", viewer.bearer), http.StatusBadRequest, "invalid_input")
	})
}
