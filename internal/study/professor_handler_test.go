package study

import (
	"context"
	"net/http"
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

	t.Run("an unknown university is invalid input", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/professors?universityId=nowhere", viewer.bearer), http.StatusBadRequest, "invalid_input")
	})
}
