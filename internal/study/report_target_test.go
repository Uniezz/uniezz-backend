package study

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestReportTarget(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("a course reports its author, name and description", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		authorID, snapshot, err := e.module.ReportTarget(ctx, "course", course.ID)
		if err != nil {
			t.Fatal(err)
		}
		if authorID != author.id || snapshot["name"] != course.Name || snapshot["description"] != course.Description {
			t.Fatalf("author %s, snapshot %#v", authorID, snapshot)
		}
	})

	t.Run("a file reports its author and title", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		file := e.file(t, author, course.ID, "Reported exam")
		authorID, snapshot, err := e.module.ReportTarget(ctx, "file", file.ID)
		if err != nil {
			t.Fatal(err)
		}
		if authorID != author.id || snapshot["title"] != "Reported exam" {
			t.Fatalf("author %s, snapshot %#v", authorID, snapshot)
		}
	})

	t.Run("a course comment reports its author and text", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		comment := decode[CourseComment](t, e.api.Post("/study/courses/"+course.ID.String()+"/comments", author.bearer, map[string]any{"content": "Rude"}), http.StatusCreated)
		authorID, snapshot, err := e.module.ReportTarget(ctx, "course_comment", comment.ID)
		if err != nil {
			t.Fatal(err)
		}
		if authorID != author.id || snapshot["content"] != "Rude" || snapshot["courseId"] != course.ID {
			t.Fatalf("author %s, snapshot %#v", authorID, snapshot)
		}
	})

	t.Run("a professor review reports its author and text", func(t *testing.T) {
		professorID := e.professor(t)
		author := e.student(t)
		review := decode[ProfessorReview](t, e.api.Put("/study/professors/"+professorID.String()+"/review", author.bearer, map[string]any{"content": "Rude"}), http.StatusOK)
		authorID, snapshot, err := e.module.ReportTarget(ctx, "professor_review", review.ID)
		if err != nil {
			t.Fatal(err)
		}
		if authorID != author.id || snapshot["content"] != "Rude" || snapshot["professorId"] != professorID {
			t.Fatalf("author %s, snapshot %#v", authorID, snapshot)
		}
	})

	t.Run("an unknown target is not found", func(t *testing.T) {
		for _, targetType := range []string{"course_comment", "professor_review"} {
			if _, _, err := e.module.ReportTarget(ctx, targetType, uuid.New()); !errors.Is(err, ErrNotFound) {
				t.Fatalf("%s: err = %v, want ErrNotFound", targetType, err)
			}
		}
		if _, _, err := e.module.ReportTarget(ctx, "file", uuid.New()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
		if _, _, err := e.module.ReportTarget(ctx, "professor", uuid.New()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}
