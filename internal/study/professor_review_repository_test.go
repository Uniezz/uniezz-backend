package study

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestProfessorReviewRepository(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("saving again replaces the student's review", func(t *testing.T) {
		author := e.student(t)
		professorID := e.professor(t)

		first, err := e.module.reviews.save(ctx, professorID, author.id, "Oral exam, three questions.")
		if err != nil {
			t.Fatalf("save: %v", err)
		}
		second, err := e.module.reviews.save(ctx, professorID, author.id, "Written exam now.")
		if err != nil {
			t.Fatalf("save again: %v", err)
		}
		if second.ID != first.ID {
			t.Errorf("id = %v, want %v", second.ID, first.ID)
		}
		if second.Content != "Written exam now." {
			t.Errorf("content = %q", second.Content)
		}
		var count int
		if err := e.pool.QueryRow(ctx, "SELECT COUNT(*) FROM professor_reviews WHERE professor_id = $1", professorID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("rows = %d, want 1", count)
		}
	})

	t.Run("an unknown professor is not found", func(t *testing.T) {
		author := e.student(t)
		if _, err := e.module.reviews.save(ctx, uuid.New(), author.id, "Hello"); !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("the list leaves out blocked students and pages newest first", func(t *testing.T) {
		viewer := e.student(t)
		blocked := e.student(t)
		professorID := e.professor(t)
		var want []uuid.UUID
		for range 3 {
			author := e.student(t)
			row, err := e.module.reviews.save(ctx, professorID, author.id, "Fair.")
			if err != nil {
				t.Fatalf("save: %v", err)
			}
			want = append([]uuid.UUID{row.ID}, want...)
		}
		if _, err := e.module.reviews.save(ctx, professorID, blocked.id, "Hidden."); err != nil {
			t.Fatalf("save: %v", err)
		}
		e.block(t, blocked.id, viewer.id)

		var got []uuid.UUID
		var after *pageCursor
		for range 10 {
			rows, err := e.module.reviews.list(ctx, professorReviewFilter{ViewerID: viewer.id, ProfessorID: professorID, After: after, Limit: 2})
			if err != nil {
				t.Fatalf("list: %v", err)
			}
			for _, row := range rows {
				got = append(got, row.ID)
			}
			if len(rows) < 2 {
				break
			}
			last := rows[len(rows)-1]
			after = &pageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
		}
		if len(got) != len(want) {
			t.Fatalf("got %d reviews, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("review %d = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("deleting a review that is gone succeeds", func(t *testing.T) {
		author := e.student(t)
		professorID := e.professor(t)
		if _, err := e.module.reviews.save(ctx, professorID, author.id, "Strict."); err != nil {
			t.Fatalf("save: %v", err)
		}
		for range 2 {
			if err := e.module.reviews.delete(ctx, professorID, author.id); err != nil {
				t.Fatalf("delete: %v", err)
			}
		}
		var count int
		if err := e.pool.QueryRow(ctx, "SELECT COUNT(*) FROM professor_reviews WHERE professor_id = $1", professorID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("rows = %d, want 0", count)
		}
	})
}
