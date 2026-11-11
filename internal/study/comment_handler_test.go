package study

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestCourseComments(t *testing.T) {
	e := newTestEnv(t)

	t.Run("come back oldest first and mark the viewer's own", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String() + "/comments"
		first := decode[CourseComment](t, e.api.Post(path, author.bearer, map[string]any{"content": "<p>Hard.</p>"}), http.StatusCreated)
		second := decode[CourseComment](t, e.api.Post(path, other.bearer, map[string]any{"content": "Fair."}), http.StatusCreated)
		if first.Content != "Hard." {
			t.Errorf("content = %q", first.Content)
		}
		page := decode[itemPage[CourseComment]](t, e.api.Get(path, author.bearer), http.StatusOK)
		if len(page.Items) != 2 || page.Items[0].ID != first.ID || page.Items[1].ID != second.ID {
			t.Fatalf("comments = %#v", page.Items)
		}
		if !page.Items[0].IsMine || page.Items[1].IsMine {
			t.Errorf("isMine = %v, %v", page.Items[0].IsMine, page.Items[1].IsMine)
		}
	})

	t.Run("on an unknown course are not found", func(t *testing.T) {
		author := e.student(t)
		path := "/study/courses/" + uuid.NewString() + "/comments"
		expectError(t, e.api.Post(path, author.bearer, map[string]any{"content": "Hi"}), http.StatusNotFound, "not_found")
		expectError(t, e.api.Get(path, author.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("of a blocked student are left out", func(t *testing.T) {
		author := e.student(t)
		blocked := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String() + "/comments"
		decode[CourseComment](t, e.api.Post(path, blocked.bearer, map[string]any{"content": "Spam"}), http.StatusCreated)
		e.block(t, author.id, blocked.id)
		page := decode[itemPage[CourseComment]](t, e.api.Get(path, author.bearer), http.StatusOK)
		if len(page.Items) != 0 {
			t.Fatalf("comments = %#v", page.Items)
		}
	})

	t.Run("only the author deletes a comment", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String() + "/comments"
		comment := decode[CourseComment](t, e.api.Post(path, author.bearer, map[string]any{"content": "Mine"}), http.StatusCreated)

		if rec := e.api.Delete("/study/comments/"+comment.ID.String(), other.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if page := decode[itemPage[CourseComment]](t, e.api.Get(path, author.bearer), http.StatusOK); len(page.Items) != 1 {
			t.Fatalf("another student's delete removed the comment")
		}
		if rec := e.api.Delete("/study/comments/"+comment.ID.String(), author.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if page := decode[itemPage[CourseComment]](t, e.api.Get(path, author.bearer), http.StatusOK); len(page.Items) != 0 {
			t.Fatalf("comment still listed")
		}
	})
}
