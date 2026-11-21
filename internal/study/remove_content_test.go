package study

import (
	"context"
	"net/http"
	"testing"
)

func TestRemoveContent(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("removing another student's file deletes it, and a repeat is a no-op", func(t *testing.T) {
		owner, author := e.student(t), e.student(t)
		course := e.course(t, owner, nil)
		file := e.file(t, author, course.ID, "Leaked exam")

		if err := e.module.RemoveFile(ctx, file.ID); err != nil {
			t.Fatalf("RemoveFile: %v", err)
		}
		if err := e.module.RemoveFile(ctx, file.ID); err != nil {
			t.Fatalf("second RemoveFile must be a no-op: %v", err)
		}
		expectError(t, e.api.Get("/study/files/"+file.ID.String(), owner.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("removing another student's course takes its files and comments, and a repeat is a no-op", func(t *testing.T) {
		owner, other := e.student(t), e.student(t)
		course := e.course(t, owner, nil)
		e.file(t, other, course.ID, "Notes")
		decode[CourseComment](t, e.api.Post("/study/courses/"+course.ID.String()+"/comments", other.bearer, map[string]any{"content": "Hard"}), http.StatusCreated)

		if err := e.module.RemoveCourse(ctx, course.ID); err != nil {
			t.Fatalf("RemoveCourse: %v", err)
		}
		if err := e.module.RemoveCourse(ctx, course.ID); err != nil {
			t.Fatalf("second RemoveCourse must be a no-op: %v", err)
		}

		var left int
		if err := e.pool.QueryRow(ctx, `
			SELECT (SELECT count(*) FROM courses WHERE id = $1)
			     + (SELECT count(*) FROM course_files WHERE course_id = $1)
			     + (SELECT count(*) FROM course_comments WHERE course_id = $1)`, course.ID).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != 0 {
			t.Errorf("%d rows of the course are left", left)
		}
	})

	t.Run("a student still cannot delete another student's course", func(t *testing.T) {
		owner, other := e.student(t), e.student(t)
		course := e.course(t, owner, nil)

		e.api.Delete("/study/courses/"+course.ID.String(), other.bearer)
		decode[Course](t, e.api.Get("/study/courses/"+course.ID.String(), owner.bearer), http.StatusOK)
	})
}
