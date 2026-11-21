package study

import (
	"context"
	"net/http"
	"testing"
)

func TestBannedStudentsContentIsHidden(t *testing.T) {
	e := newTestEnv(t)
	owner, banned := e.student(t), e.student(t)
	course := e.course(t, owner, nil)
	file := e.file(t, banned, course.ID, "Leaked exam")
	comments := "/study/courses/" + course.ID.String() + "/comments"
	decode[CourseComment](t, e.api.Post(comments, banned.bearer, map[string]any{"content": "Spam"}), http.StatusCreated)

	if _, err := e.pool.Exec(context.Background(), "UPDATE users SET banned_at = NOW() WHERE id = $1", banned.id); err != nil {
		t.Fatal(err)
	}

	t.Run("files are hidden from lists and not found directly", func(t *testing.T) {
		if list := decode[itemPage[CourseFile]](t, e.api.Get("/study/courses/"+course.ID.String()+"/files", owner.bearer), http.StatusOK); len(list.Items) != 0 {
			t.Errorf("files = %#v", list.Items)
		}
		expectError(t, e.api.Get("/study/files/"+file.ID.String(), owner.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("course comments are hidden", func(t *testing.T) {
		if page := decode[itemPage[CourseComment]](t, e.api.Get(comments, owner.bearer), http.StatusOK); len(page.Items) != 0 {
			t.Errorf("comments = %#v", page.Items)
		}
	})
}
