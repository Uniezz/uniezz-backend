package study

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestForgetUser(t *testing.T) {
	e := newTestEnv(t)

	t.Run("removes files, comments and ratings so the user row can go", func(t *testing.T) {
		author := e.student(t)
		leaving := e.student(t)
		course := e.course(t, author, nil)
		own := e.course(t, leaving, nil)
		file := e.file(t, leaving, course.ID, "Old exam")
		decode[CourseComment](t, e.api.Post("/study/courses/"+course.ID.String()+"/comments", leaving.bearer, map[string]any{"content": "Bye"}), http.StatusCreated)
		decode[Course](t, e.api.Put("/study/courses/"+course.ID.String()+"/rating", author.bearer, map[string]any{"rating": 2}), http.StatusOK)
		decode[Course](t, e.api.Put("/study/courses/"+course.ID.String()+"/rating", leaving.bearer, map[string]any{"rating": 4}), http.StatusOK)

		ctx := context.Background()
		if err := e.module.ForgetUser(ctx, leaving.id); err != nil {
			t.Fatal(err)
		}

		rated := decode[Course](t, e.api.Get("/study/courses/"+course.ID.String(), author.bearer), http.StatusOK)
		if rated.RatingCount != 1 || rated.RatingAverage != 2 {
			t.Errorf("rating after forget = %#v", rated)
		}
		expectError(t, e.api.Get("/study/files/"+file.ID.String(), author.bearer), http.StatusNotFound, "not_found")
		if page := decode[itemPage[CourseComment]](t, e.api.Get("/study/courses/"+course.ID.String()+"/comments", author.bearer), http.StatusOK); len(page.Items) != 0 {
			t.Errorf("comments after forget = %#v", page.Items)
		}

		if _, err := e.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", leaving.id); err != nil {
			t.Fatalf("delete user after forget: %v", err)
		}
		var authorID *uuid.UUID
		if err := e.pool.QueryRow(ctx, "SELECT author_id FROM courses WHERE id = $1", own.ID).Scan(&authorID); err != nil {
			t.Fatalf("the user's own course is gone: %v", err)
		}
		if authorID != nil {
			t.Errorf("course author = %v, want null", authorID)
		}
	})
}
