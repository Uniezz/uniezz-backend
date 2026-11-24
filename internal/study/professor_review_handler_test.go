package study

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestProfessorReviews(t *testing.T) {
	e := newTestEnv(t)

	t.Run("reviewing again replaces the text and keeps the review", func(t *testing.T) {
		author := e.student(t)
		professorID := e.professor(t)
		path := "/study/professors/" + professorID.String() + "/review"

		first := decode[ProfessorReview](t, e.api.Put(path, author.bearer, map[string]any{"content": "<b>Oral</b> exam."}), http.StatusOK)
		if first.Content != "Oral exam." || !first.IsMine || first.ProfessorID != professorID {
			t.Fatalf("review = %#v", first)
		}
		second := decode[ProfessorReview](t, e.api.Put(path, author.bearer, map[string]any{"content": "Written exam."}), http.StatusOK)
		if second.ID != first.ID || second.Content != "Written exam." {
			t.Fatalf("review = %#v, want id %v", second, first.ID)
		}
		page := decode[itemPage[ProfessorReview]](t, e.api.Get(path+"s", author.bearer), http.StatusOK)
		if len(page.Items) != 1 || page.Items[0].Content != "Written exam." {
			t.Fatalf("reviews = %#v", page.Items)
		}
	})

	t.Run("come back newest first and mark the viewer's own", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		professorID := e.professor(t)
		path := "/study/professors/" + professorID.String() + "/review"
		first := decode[ProfessorReview](t, e.api.Put(path, author.bearer, map[string]any{"content": "Fair."}), http.StatusOK)
		second := decode[ProfessorReview](t, e.api.Put(path, other.bearer, map[string]any{"content": "Strict."}), http.StatusOK)

		page := decode[itemPage[ProfessorReview]](t, e.api.Get(path+"s", author.bearer), http.StatusOK)
		if len(page.Items) != 2 || page.Items[0].ID != second.ID || page.Items[1].ID != first.ID {
			t.Fatalf("reviews = %#v", page.Items)
		}
		if page.Items[0].IsMine || !page.Items[1].IsMine {
			t.Errorf("isMine = %v, %v", page.Items[0].IsMine, page.Items[1].IsMine)
		}
	})

	t.Run("empty or too long text is invalid", func(t *testing.T) {
		author := e.student(t)
		path := "/study/professors/" + e.professor(t).String() + "/review"
		expectError(t, e.api.Put(path, author.bearer, map[string]any{"content": "<p></p>"}), http.StatusBadRequest, "invalid_input")
		expectError(t, e.api.Put(path, author.bearer, map[string]any{"content": strings.Repeat("a", 1001)}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("an unknown or departed professor is not found", func(t *testing.T) {
		author := e.student(t)
		departed := e.professor(t)
		if _, err := e.pool.Exec(context.Background(), "UPDATE professors SET active = FALSE WHERE id = $1", departed); err != nil {
			t.Fatal(err)
		}
		for _, id := range []uuid.UUID{uuid.New(), departed} {
			path := "/study/professors/" + id.String() + "/review"
			expectError(t, e.api.Put(path, author.bearer, map[string]any{"content": "Hi"}), http.StatusNotFound, "not_found")
			expectError(t, e.api.Get(path+"s", author.bearer), http.StatusNotFound, "not_found")
		}
	})

	t.Run("reviews of a blocked student are left out", func(t *testing.T) {
		viewer := e.student(t)
		blocked := e.student(t)
		path := "/study/professors/" + e.professor(t).String() + "/review"
		decode[ProfessorReview](t, e.api.Put(path, blocked.bearer, map[string]any{"content": "Spam"}), http.StatusOK)
		e.block(t, viewer.id, blocked.id)
		if page := decode[itemPage[ProfessorReview]](t, e.api.Get(path+"s", viewer.bearer), http.StatusOK); len(page.Items) != 0 {
			t.Fatalf("reviews = %#v", page.Items)
		}
	})

	t.Run("deleting removes only the caller's review and succeeds twice", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		path := "/study/professors/" + e.professor(t).String() + "/review"
		decode[ProfessorReview](t, e.api.Put(path, author.bearer, map[string]any{"content": "Mine"}), http.StatusOK)
		decode[ProfessorReview](t, e.api.Put(path, other.bearer, map[string]any{"content": "Theirs"}), http.StatusOK)

		for range 2 {
			if rec := e.api.Delete(path, author.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d", rec.Code)
			}
		}
		page := decode[itemPage[ProfessorReview]](t, e.api.Get(path+"s", author.bearer), http.StatusOK)
		if len(page.Items) != 1 || page.Items[0].Content != "Theirs" {
			t.Fatalf("reviews = %#v", page.Items)
		}
	})
}
