package study

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestCourseEndpointsNeedSession(t *testing.T) {
	e := newTestEnv(t)
	id := uuid.NewString()
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"create course", http.MethodPost, "/study/courses"},
		{"list courses", http.MethodGet, "/study/courses"},
		{"get course", http.MethodGet, "/study/courses/" + id},
		{"update course", http.MethodPatch, "/study/courses/" + id},
		{"delete course", http.MethodDelete, "/study/courses/" + id},
		{"rate course", http.MethodPut, "/study/courses/" + id + "/rating"},
		{"add file", http.MethodPost, "/study/courses/" + id + "/files"},
		{"list course files", http.MethodGet, "/study/courses/" + id + "/files"},
		{"search files", http.MethodGet, "/study/files"},
		{"get file", http.MethodGet, "/study/files/" + id},
		{"delete file", http.MethodDelete, "/study/files/" + id},
		{"add comment", http.MethodPost, "/study/courses/" + id + "/comments"},
		{"list comments", http.MethodGet, "/study/courses/" + id + "/comments"},
		{"delete comment", http.MethodDelete, "/study/comments/" + id},
		{"list professors", http.MethodGet, "/study/professors"},
		{"get professor", http.MethodGet, "/study/professors/" + id},
		{"rate professor", http.MethodPut, "/study/professors/" + id + "/rating"},
	}
	for _, tc := range cases {
		t.Run(tc.name+" without a session is 401", func(t *testing.T) {
			if rec := e.api.Do(tc.method, tc.path, map[string]any{}); rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateCourse(t *testing.T) {
	e := newTestEnv(t)

	t.Run("stores clean text and reads back the same", func(t *testing.T) {
		author := e.student(t)
		created := e.course(t, author, map[string]any{"name": "<b>Calculus</b> I"})
		if created.Name != "Calculus I" || created.Difficulty != 3 || created.UniversityID != "umcs" || !created.IsMine {
			t.Fatalf("created = %#v", created)
		}
		got := decode[Course](t, e.api.Get("/study/courses/"+created.ID.String(), author.bearer), http.StatusOK)
		if got != created {
			t.Fatalf("get = %#v, created = %#v", got, created)
		}
		other := e.student(t)
		seen := decode[Course](t, e.api.Get("/study/courses/"+created.ID.String(), other.bearer), http.StatusOK)
		if seen.IsMine {
			t.Error("isMine is true for another student")
		}
	})

	t.Run("rejects a short name, a bad difficulty and an unknown university", func(t *testing.T) {
		author := e.student(t)
		base := map[string]any{"name": "Calculus", "description": "x", "difficulty": 3, "universityId": "umcs", "faculty": "Maths", "subject": "Analysis", "semester": 1}
		cases := []struct {
			name  string
			field string
			value any
		}{
			{"name under 3 characters after cleaning", "name", "<i>AB</i>"},
			{"difficulty above 5", "difficulty", 6},
			{"unknown university", "universityId", "nowhere"},
		}
		for _, tc := range cases {
			body := map[string]any{}
			for k, v := range base {
				body[k] = v
			}
			body[tc.field] = tc.value
			t.Run(tc.name, func(t *testing.T) {
				expectError(t, e.api.Post("/study/courses", author.bearer, body), http.StatusBadRequest, "invalid_input")
			})
		}
	})

	t.Run("unknown course is not found", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/courses/"+uuid.NewString(), viewer.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("the 31st create in an hour is rate limited", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		for i := 1; i < createLimit; i++ {
			decode[CourseComment](t, e.api.Post("/study/courses/"+course.ID.String()+"/comments", author.bearer, map[string]any{"content": "ok"}), http.StatusCreated)
		}
		expectError(t, e.api.Post("/study/courses/"+course.ID.String()+"/comments", author.bearer, map[string]any{"content": "one more"}), http.StatusTooManyRequests, "rate_limited")
	})
}

func TestListCourses(t *testing.T) {
	e := newTestEnv(t)

	t.Run("filters combine and short filter values work", func(t *testing.T) {
		author := e.student(t)
		subject := "AI " + uuid.NewString()[:6]
		match := e.course(t, author, map[string]any{"subject": subject, "semester": 2})
		e.course(t, author, map[string]any{"subject": subject, "semester": 3})

		page := decode[itemPage[Course]](t, e.api.Get("/study/courses?universityId=umcs&subject="+url.QueryEscape(subject)+"&semester=2", author.bearer), http.StatusOK)
		if len(page.Items) != 1 || page.Items[0].ID != match.ID {
			t.Fatalf("courses = %#v", page.Items)
		}
	})

	t.Run("pages follow the cursor without repeats", func(t *testing.T) {
		author := e.student(t)
		subject := "Paged " + uuid.NewString()[:6]
		for range 3 {
			e.course(t, author, map[string]any{"subject": subject})
		}
		first := decode[itemPage[Course]](t, e.api.Get("/study/courses?limit=2&subject="+url.QueryEscape(subject), author.bearer), http.StatusOK)
		if len(first.Items) != 2 || first.NextCursor == nil {
			t.Fatalf("first page = %#v", first)
		}
		second := decode[itemPage[Course]](t, e.api.Get("/study/courses?limit=2&subject="+url.QueryEscape(subject)+"&cursor="+*first.NextCursor, author.bearer), http.StatusOK)
		if len(second.Items) != 1 || second.NextCursor != nil || second.Items[0].ID == first.Items[0].ID || second.Items[0].ID == first.Items[1].ID {
			t.Fatalf("second page = %#v", second)
		}
	})

	t.Run("a broken cursor is rejected", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/courses?cursor=@@@", viewer.bearer), http.StatusBadRequest, "invalid_cursor")
	})
}

func TestUpdateCourse(t *testing.T) {
	e := newTestEnv(t)

	t.Run("the author changes only the fields sent", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		updated := decode[Course](t, e.api.Patch("/study/courses/"+course.ID.String(), author.bearer, map[string]any{"difficulty": 5}), http.StatusOK)
		if updated.Difficulty != 5 || updated.Name != course.Name || updated.Description != course.Description {
			t.Fatalf("updated = %#v", updated)
		}
	})

	t.Run("another student gets not found", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		expectError(t, e.api.Patch("/study/courses/"+course.ID.String(), other.bearer, map[string]any{"name": "Hijacked"}), http.StatusNotFound, "not_found")
	})
}

func TestRateCourse(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a second rating replaces the first", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String() + "/rating"
		first := decode[Course](t, e.api.Put(path, author.bearer, map[string]any{"rating": 2}), http.StatusOK)
		if first.RatingCount != 1 || first.RatingAverage != 2 {
			t.Fatalf("first = %#v", first)
		}
		second := decode[Course](t, e.api.Put(path, author.bearer, map[string]any{"rating": 4}), http.StatusOK)
		if second.RatingCount != 1 || second.RatingAverage != 4 {
			t.Fatalf("second = %#v", second)
		}
		both := decode[Course](t, e.api.Put(path, other.bearer, map[string]any{"rating": 2}), http.StatusOK)
		if both.RatingCount != 2 || both.RatingAverage != 3 {
			t.Fatalf("both = %#v", both)
		}
	})

	t.Run("ratings sent at once keep the counters equal to the rows", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		raters := make([]student, 5)
		for i := range raters {
			raters[i] = e.student(t)
		}
		path := "/study/courses/" + course.ID.String() + "/rating"
		var wg sync.WaitGroup
		for i, rater := range raters {
			for _, rating := range []int{1 + i%5, 5 - i%5} {
				wg.Go(func() { e.api.Put(path, rater.bearer, map[string]any{"rating": rating}) })
			}
		}
		wg.Wait()

		var count, sum, rows, rowSum int
		err := e.pool.QueryRow(context.Background(), `
			SELECT c.rating_count, c.rating_sum, COUNT(r.user_id), COALESCE(SUM(r.rating), 0)
			FROM courses c LEFT JOIN course_ratings r ON r.course_id = c.id
			WHERE c.id = $1 GROUP BY c.id`, course.ID).Scan(&count, &sum, &rows, &rowSum)
		if err != nil {
			t.Fatal(err)
		}
		if count != rows || sum != rowSum || rows != len(raters) {
			t.Fatalf("counters %d/%d, rows %d/%d", count, sum, rows, rowSum)
		}
	})

	t.Run("an unknown course is not found", func(t *testing.T) {
		rater := e.student(t)
		expectError(t, e.api.Put("/study/courses/"+uuid.NewString()+"/rating", rater.bearer, map[string]any{"rating": 3}), http.StatusNotFound, "not_found")
	})
}

func TestDeleteCourse(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	count := func(t *testing.T, query string, id uuid.UUID) int {
		t.Helper()
		var n int
		if err := e.pool.QueryRow(ctx, query, id).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		return n
	}

	t.Run("the author deletes the course with every student's files, comments and ratings", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String()
		file := e.file(t, other, course.ID, "Exam 2025")
		decode[CourseComment](t, e.api.Post(path+"/comments", other.bearer, map[string]any{"content": "Fair."}), http.StatusCreated)
		decode[Course](t, e.api.Put(path+"/rating", other.bearer, map[string]any{"rating": 4}), http.StatusOK)
		var mediaID uuid.UUID
		if err := e.pool.QueryRow(ctx, "SELECT media_id FROM course_files WHERE id = $1", file.ID).Scan(&mediaID); err != nil {
			t.Fatalf("read file media: %v", err)
		}

		if rec := e.api.Delete(path, author.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}

		expectError(t, e.api.Get(path, author.bearer), http.StatusNotFound, "not_found")
		for _, query := range []string{
			"SELECT count(*) FROM course_files WHERE course_id = $1",
			"SELECT count(*) FROM course_comments WHERE course_id = $1",
			"SELECT count(*) FROM course_ratings WHERE course_id = $1",
		} {
			if n := count(t, query, course.ID); n != 0 {
				t.Errorf("%s = %d, want 0", query, n)
			}
		}
		var status string
		if err := e.pool.QueryRow(ctx, "SELECT status FROM media WHERE id = $1", mediaID).Scan(&status); err != nil {
			t.Fatalf("read media status: %v", err)
		}
		if status != "deleting" {
			t.Errorf("media status = %q, want deleting", status)
		}
	})

	t.Run("another student's delete leaves the course as it is", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String()

		if rec := e.api.Delete(path, other.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		decode[Course](t, e.api.Get(path, author.bearer), http.StatusOK)
	})

	t.Run("deleting a course that is already gone succeeds", func(t *testing.T) {
		author := e.student(t)
		if rec := e.api.Delete("/study/courses/"+uuid.NewString(), author.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("comments sent while the course is deleted never break the delete", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		path := "/study/courses/" + course.ID.String()

		const commenters = 10
		var wg sync.WaitGroup
		codes := make([]int, commenters)
		var deleteCode int
		wg.Add(commenters + 1)
		for i := range commenters {
			go func() {
				defer wg.Done()
				codes[i] = e.api.Post(path+"/comments", other.bearer, map[string]any{"content": "Racing."}).Code
			}()
		}
		go func() {
			defer wg.Done()
			deleteCode = e.api.Delete(path, author.bearer).Code
		}()
		wg.Wait()

		if deleteCode != http.StatusNoContent {
			t.Fatalf("delete = %d, want 204", deleteCode)
		}
		for i, code := range codes {
			if code != http.StatusCreated && code != http.StatusNotFound {
				t.Errorf("comment %d = %d, want 201 or 404", i, code)
			}
		}
		if n := count(t, "SELECT count(*) FROM courses WHERE id = $1", course.ID); n != 0 {
			t.Errorf("course rows = %d, want 0", n)
		}
		if n := count(t, "SELECT count(*) FROM course_comments WHERE course_id = $1", course.ID); n != 0 {
			t.Errorf("comment rows = %d, want 0", n)
		}
	})
}
