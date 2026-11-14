package study

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) professor(t *testing.T) uuid.UUID {
	t.Helper()
	externalID := uuid.New()
	var id uuid.UUID
	err := e.pool.QueryRow(context.Background(), `
		INSERT INTO professors (external_id, radon_id, name, university_id)
		VALUES ($1, 'R-test', $2, 'wsei') RETURNING id`, externalID, "Test "+externalID.String()).Scan(&id)
	if err != nil {
		t.Fatalf("insert professor: %v", err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM professors WHERE id = $1", id) })
	return id
}

func listed[T any](t *testing.T, e *testEnv, path string, viewer student, id uuid.UUID, idOf func(T) uuid.UUID) T {
	t.Helper()
	cursor := ""
	for range 500 {
		page := decode[itemPage[T]](t, e.api.Get(path+"&limit=50"+cursor, viewer.bearer), http.StatusOK)
		for _, item := range page.Items {
			if idOf(item) == id {
				return item
			}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = "&cursor=" + *page.NextCursor
	}
	t.Fatalf("%s is not listed", id)
	var zero T
	return zero
}

func ratingOf(r *int) any {
	if r == nil {
		return nil
	}
	return *r
}

func TestCourseMyRating(t *testing.T) {
	e := newTestEnv(t)
	courseID := func(c Course) uuid.UUID { return c.ID }

	t.Run("each student sees only their own rating next to the shared average", func(t *testing.T) {
		author := e.student(t)
		rater := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String()
		listPath := "/study/courses?subject=" + course.Subject

		if course.MyRating != nil {
			t.Errorf("created course myRating = %v, want null", *course.MyRating)
		}
		if got := decode[Course](t, e.api.Get(path, rater.bearer), http.StatusOK).MyRating; got != nil {
			t.Errorf("before rating myRating = %v, want null", *got)
		}

		rated := decode[Course](t, e.api.Put(path+"/rating", rater.bearer, map[string]any{"rating": 2}), http.StatusOK)
		decode[Course](t, e.api.Put(path+"/rating", other.bearer, map[string]any{"rating": 4}), http.StatusOK)

		if ratingOf(rated.MyRating) != 2 {
			t.Errorf("rate response myRating = %v, want 2", ratingOf(rated.MyRating))
		}
		got := decode[Course](t, e.api.Get(path, rater.bearer), http.StatusOK)
		if ratingOf(got.MyRating) != 2 || got.RatingCount != 2 || got.RatingAverage != 3 {
			t.Errorf("get = myRating %v, count %d, average %v; want 2, 2, 3", ratingOf(got.MyRating), got.RatingCount, got.RatingAverage)
		}
		if r := listed(t, e, listPath, rater, course.ID, courseID).MyRating; ratingOf(r) != 2 {
			t.Errorf("list myRating = %v, want 2", ratingOf(r))
		}
		if r := listed(t, e, listPath, author, course.ID, courseID).MyRating; r != nil {
			t.Errorf("author who did not rate sees myRating %v, want null", *r)
		}
	})

	t.Run("the author's update returns their own rating", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String()
		decode[Course](t, e.api.Put(path+"/rating", author.bearer, map[string]any{"rating": 5}), http.StatusOK)

		updated := decode[Course](t, e.api.Patch(path, author.bearer, map[string]any{"difficulty": 1}), http.StatusOK)
		if ratingOf(updated.MyRating) != 5 {
			t.Errorf("update response myRating = %v, want 5", ratingOf(updated.MyRating))
		}
	})
}

func TestProfessorMyRating(t *testing.T) {
	e := newTestEnv(t)
	professorID := func(p Professor) uuid.UUID { return p.ID }

	t.Run("each student sees only their own rating next to the shared average", func(t *testing.T) {
		rater := e.student(t)
		other := e.student(t)
		id := e.professor(t)
		path := "/study/professors/" + id.String()
		listPath := "/study/professors?universityId=wsei"

		if got := decode[Professor](t, e.api.Get(path, rater.bearer), http.StatusOK).MyRating; got != nil {
			t.Errorf("before rating myRating = %v, want null", *got)
		}

		rated := decode[Professor](t, e.api.Put(path+"/rating", rater.bearer, map[string]any{"rating": 5}), http.StatusOK)
		decode[Professor](t, e.api.Put(path+"/rating", other.bearer, map[string]any{"rating": 3}), http.StatusOK)

		if ratingOf(rated.MyRating) != 5 {
			t.Errorf("rate response myRating = %v, want 5", ratingOf(rated.MyRating))
		}
		got := decode[Professor](t, e.api.Get(path, rater.bearer), http.StatusOK)
		if ratingOf(got.MyRating) != 5 || got.RatingCount != 2 || got.RatingAverage != 4 {
			t.Errorf("get = myRating %v, count %d, average %v; want 5, 2, 4", ratingOf(got.MyRating), got.RatingCount, got.RatingAverage)
		}
		if r := listed(t, e, listPath, rater, id, professorID).MyRating; ratingOf(r) != 5 {
			t.Errorf("list myRating = %v, want 5", ratingOf(r))
		}
		if r := listed(t, e, listPath, e.student(t), id, professorID).MyRating; r != nil {
			t.Errorf("student who did not rate sees myRating %v, want null", *r)
		}
	})

	t.Run("rating again replaces the student's rating", func(t *testing.T) {
		rater := e.student(t)
		path := "/study/professors/" + e.professor(t).String()
		decode[Professor](t, e.api.Put(path+"/rating", rater.bearer, map[string]any{"rating": 1}), http.StatusOK)

		rated := decode[Professor](t, e.api.Put(path+"/rating", rater.bearer, map[string]any{"rating": 4}), http.StatusOK)
		if ratingOf(rated.MyRating) != 4 || rated.RatingCount != 1 {
			t.Errorf("myRating %v, count %d; want 4, 1", ratingOf(rated.MyRating), rated.RatingCount)
		}
	})
}
