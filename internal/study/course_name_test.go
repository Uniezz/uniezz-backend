package study

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestCourseNames(t *testing.T) {
	e := newTestEnv(t)

	create := func(t *testing.T, author student, faculty uuid.UUID, name string, confirmNew bool) *httptest.ResponseRecorder {
		t.Helper()
		rec := e.api.Post("/study/courses", author.bearer, map[string]any{
			"name": name, "description": "Opis.", "difficulty": 3, "facultyId": faculty, "confirmNew": confirmNew,
		})
		if rec.Code == http.StatusCreated {
			e.cleanupCourse(t, decode[Course](t, rec, http.StatusCreated).ID)
		}
		return rec
	}

	t.Run("the same name in other case and accents is a conflict even when confirmed", func(t *testing.T) {
		author := e.student(t)
		faculty := e.newFaculty(t)
		create(t, author, faculty, "Analiza matematyczna", false)

		expectError(t, create(t, author, faculty, "ANALIZA  matematyczna", true), http.StatusConflict, "course_exists")
		expectError(t, create(t, author, faculty, "Ąnaliza matematyczna", true), http.StatusConflict, "course_exists")
		if n := e.countRows(t, "SELECT count(*) FROM courses WHERE faculty_id = $1", faculty); n != 1 {
			t.Errorf("courses = %d, want 1", n)
		}
	})

	t.Run("a similar name needs confirmation", func(t *testing.T) {
		author := e.student(t)
		faculty := e.newFaculty(t)
		create(t, author, faculty, "Analiza matematyczna", false)

		expectError(t, create(t, author, faculty, "Analza matematyczna", false), http.StatusConflict, "similar_courses")
		decode[Course](t, create(t, author, faculty, "Analiza matematyczna 2", true), http.StatusCreated)
	})

	t.Run("a different name and the same name in another faculty need no confirmation", func(t *testing.T) {
		author := e.student(t)
		faculty := e.newFaculty(t)
		create(t, author, faculty, "Analiza matematyczna", false)

		decode[Course](t, create(t, author, faculty, "Algebra liniowa", false), http.StatusCreated)
		decode[Course](t, create(t, author, e.newFaculty(t), "Analiza matematyczna", false), http.StatusCreated)
	})

	t.Run("renaming a course to a name the faculty has is a conflict", func(t *testing.T) {
		author := e.student(t)
		faculty := e.newFaculty(t)
		create(t, author, faculty, "Bazy danych", false)
		other := decode[Course](t, create(t, author, faculty, "Sieci komputerowe", false), http.StatusCreated)

		expectError(t, e.api.Patch("/study/courses/"+other.ID.String(), author.bearer, map[string]any{"name": "bazy danych"}), http.StatusConflict, "course_exists")
	})

	t.Run("two students creating the same course at once store it once", func(t *testing.T) {
		faculty := e.newFaculty(t)
		authors := []student{e.student(t), e.student(t)}
		codes := make([]int, len(authors))
		var wg sync.WaitGroup
		for i, author := range authors {
			wg.Go(func() {
				codes[i] = e.api.Post("/study/courses", author.bearer, map[string]any{
					"name": "Fizyka kwantowa", "description": "Opis.", "difficulty": 3, "facultyId": faculty, "confirmNew": true,
				}).Code
			})
		}
		wg.Wait()
		t.Cleanup(func() {
			_, _ = e.pool.Exec(context.Background(), "DELETE FROM courses WHERE faculty_id = $1", faculty)
		})

		created, conflicts := 0, 0
		for _, code := range codes {
			switch code {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
				conflicts++
			}
		}
		if created != 1 || conflicts != 1 {
			t.Errorf("codes = %v, want one 201 and one 409", codes)
		}
		if n := e.countRows(t, "SELECT count(*) FROM courses WHERE faculty_id = $1", faculty); n != 1 {
			t.Errorf("courses = %d, want 1", n)
		}
	})
}

func TestListSimilarCourses(t *testing.T) {
	e := newTestEnv(t)

	similar := func(t *testing.T, viewer student, faculty uuid.UUID, name string) []Course {
		t.Helper()
		path := "/study/courses/similar?facultyId=" + faculty.String() + "&name=" + url.QueryEscape(name)
		return decode[struct{ Items []Course }](t, e.api.Get(path, viewer.bearer), http.StatusOK).Items
	}

	t.Run("lists the most similar courses of the faculty first", func(t *testing.T) {
		author := e.student(t)
		faculty := e.newFaculty(t)
		exact := e.course(t, author, map[string]any{"name": "Analiza matematyczna", "facultyId": faculty})
		close := e.course(t, author, map[string]any{"name": "Analiza matematyczna 2", "facultyId": faculty})
		e.course(t, author, map[string]any{"name": "Algebra liniowa", "facultyId": faculty})
		e.course(t, author, map[string]any{"name": "Analiza matematyczna"})

		got := similar(t, author, faculty, "analiza matematyczna")
		if len(got) != 2 || got[0].ID != exact.ID || got[1].ID != close.ID {
			t.Fatalf("similar = %+v", got)
		}
	})

	t.Run("an unknown faculty has no similar courses", func(t *testing.T) {
		viewer := e.student(t)
		if got := similar(t, viewer, uuid.New(), "Analiza"); len(got) != 0 {
			t.Fatalf("similar = %+v", got)
		}
	})

	t.Run("a name under 3 characters is invalid input", func(t *testing.T) {
		viewer := e.student(t)
		expectError(t, e.api.Get("/study/courses/similar?facultyId="+e.faculty.String()+"&name=ab", viewer.bearer), http.StatusBadRequest, "invalid_input")
	})
}
