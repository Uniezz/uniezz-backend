package study

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestBlockedText(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a course with blocked text in any field is rejected and not stored", func(t *testing.T) {
		author := e.student(t)
		for field, value := range map[string]string{
			"name":        "Kurwa analiza",
			"description": "Limits, k u r w a, derivatives.",
		} {
			body := map[string]any{
				"name":        "Calculus " + uuid.NewString()[:8],
				"description": "Limits and derivatives.",
				"difficulty":  3,
				"facultyId":   e.faculty,
			}
			body[field] = value
			expectError(t, e.api.Post("/study/courses", author.bearer, body), http.StatusBadRequest, "content_not_allowed")
		}
		if n := e.countRows(t, "SELECT count(*) FROM courses WHERE author_id = $1", author.id); n != 0 {
			t.Errorf("stored %d courses, want 0", n)
		}
	})

	t.Run("editing a course into blocked text is rejected and the course stays unchanged", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String()

		expectError(t, e.api.Patch(path, author.bearer, map[string]any{"name": "Kurwa"}), http.StatusBadRequest, "content_not_allowed")
		expectError(t, e.api.Patch(path, author.bearer, map[string]any{"description": "sh1t"}), http.StatusBadRequest, "content_not_allowed")

		var name, description string
		if err := e.pool.QueryRow(context.Background(), "SELECT name, description FROM courses WHERE id = $1", course.ID).Scan(&name, &description); err != nil {
			t.Fatalf("read course: %v", err)
		}
		if name != course.Name || description != "Limits and derivatives." {
			t.Errorf("course changed: %q, %q", name, description)
		}
	})

	t.Run("a file with a blocked title is rejected and not stored", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)

		body := map[string]any{"mediaId": e.upload(t, author.id), "title": "Egzamin kurwa 2024", "type": "exam"}
		expectError(t, e.api.Post("/study/courses/"+course.ID.String()+"/files", author.bearer, body), http.StatusBadRequest, "content_not_allowed")

		if n := e.countRows(t, "SELECT count(*) FROM course_files WHERE course_id = $1", course.ID); n != 0 {
			t.Errorf("stored %d files, want 0", n)
		}
	})

	t.Run("a blocked course comment is rejected and not stored", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)

		rec := e.api.Post("/study/courses/"+course.ID.String()+"/comments", author.bearer, map[string]any{"content": "f u c k this"})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")

		if n := e.countRows(t, "SELECT count(*) FROM course_comments WHERE course_id = $1", course.ID); n != 0 {
			t.Errorf("stored %d comments, want 0", n)
		}
	})

	t.Run("a blocked professor review is rejected and not stored", func(t *testing.T) {
		author := e.student(t)
		professorID := e.professor(t)

		rec := e.api.Put("/study/professors/"+professorID.String()+"/review", author.bearer, map[string]any{"content": "Egzamin to sh1t"})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")

		if n := e.countRows(t, "SELECT count(*) FROM professor_reviews WHERE professor_id = $1", professorID); n != 0 {
			t.Errorf("stored %d reviews, want 0", n)
		}
	})

	t.Run("ordinary course text in several languages is accepted", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, map[string]any{"description": "Granice i pochodne. Межі та похідні. Límites y derivadas."})

		e.file(t, author, course.ID, "Egzamin 2024 z rozwiązaniami")
		rec := e.api.Post("/study/courses/"+course.ID.String()+"/comments", author.bearer, map[string]any{"content": "Dobre notatki, dzięki!"})
		if rec.Code != http.StatusCreated {
			t.Errorf("comment: expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
		rec = e.api.Put("/study/professors/"+e.professor(t).String()+"/review", author.bearer, map[string]any{"content": "Ustny egzamin, trzy pytania z listy."})
		if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
			t.Errorf("review: expected 200 or 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}
