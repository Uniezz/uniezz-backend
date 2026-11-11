package study

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestAddCourseFile(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a pending upload becomes a file of the course", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		file := e.file(t, author, course.ID, "Midterm 2024")
		if file.Title != "Midterm 2024" || file.Type != FileTypeExam || file.AuthorID != author.id || file.CourseID != course.ID || file.URL == "" {
			t.Fatalf("file = %#v", file)
		}
		got := decode[CourseFile](t, e.api.Get("/study/files/"+file.ID.String(), author.bearer), http.StatusOK)
		if got.ID != file.ID || got.URL != file.URL {
			t.Fatalf("get = %#v", got)
		}
	})

	t.Run("an upload of another student is invalid media", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		body := map[string]any{"mediaId": e.upload(t, other.id), "title": "Stolen", "type": "exam"}
		expectError(t, e.api.Post("/study/courses/"+course.ID.String()+"/files", author.bearer, body), http.StatusBadRequest, "invalid_media")
	})

	t.Run("an upload of another kind is invalid media", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		id := uuid.New()
		_, err := e.pool.Exec(context.Background(), `
			INSERT INTO media (id, owner_user_id, kind, object_key, content_type, size_bytes)
			VALUES ($1, $2, 'feed', $3, 'image/png', 10)`, id, author.id, "feed/"+id.String()+".png")
		if err != nil {
			t.Fatalf("insert media: %v", err)
		}
		body := map[string]any{"mediaId": id, "title": "Not an exam", "type": "exam"}
		expectError(t, e.api.Post("/study/courses/"+course.ID.String()+"/files", author.bearer, body), http.StatusBadRequest, "invalid_media")
	})

	t.Run("an unknown course is not found and the upload stays pending", func(t *testing.T) {
		author := e.student(t)
		mediaID := e.upload(t, author.id)
		body := map[string]any{"mediaId": mediaID, "title": "Lost", "type": "notes"}
		expectError(t, e.api.Post("/study/courses/"+uuid.NewString()+"/files", author.bearer, body), http.StatusNotFound, "not_found")
		var status string
		if err := e.pool.QueryRow(context.Background(), "SELECT status FROM media WHERE id = $1", mediaID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "pending" {
			t.Errorf("media status = %s, want pending", status)
		}
	})

	t.Run("an unknown type is invalid input", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		body := map[string]any{"mediaId": e.upload(t, author.id), "title": "Slides", "type": "slides"}
		expectError(t, e.api.Post("/study/courses/"+course.ID.String()+"/files", author.bearer, body), http.StatusBadRequest, "invalid_input")
	})
}

func TestSearchFiles(t *testing.T) {
	e := newTestEnv(t)
	author := e.student(t)
	other := e.student(t)
	marker := "zq" + uuid.NewString()[:6]
	course := e.course(t, author, map[string]any{"subject": "Algebra " + marker})
	file := e.file(t, author, course.ID, "Midterm "+marker)

	t.Run("matches the title or the course subject", func(t *testing.T) {
		byTitle := decode[itemPage[CourseFile]](t, e.api.Get("/study/files?q=midterm+"+marker, other.bearer), http.StatusOK)
		if len(byTitle.Items) != 1 || byTitle.Items[0].ID != file.ID {
			t.Fatalf("by title = %#v", byTitle.Items)
		}
		bySubject := decode[itemPage[CourseFile]](t, e.api.Get("/study/files?q=algebra+"+marker, other.bearer), http.StatusOK)
		if len(bySubject.Items) != 1 || bySubject.Items[0].ID != file.ID {
			t.Fatalf("by subject = %#v", bySubject.Items)
		}
	})

	t.Run("course filters apply", func(t *testing.T) {
		miss := decode[itemPage[CourseFile]](t, e.api.Get("/study/files?q="+marker+"&universityId=kul", other.bearer), http.StatusOK)
		if len(miss.Items) != 0 {
			t.Fatalf("filtered = %#v", miss.Items)
		}
	})

	t.Run("mine lists only the student's own uploads", func(t *testing.T) {
		mine := decode[itemPage[CourseFile]](t, e.api.Get("/study/files?mine=true&q="+marker, author.bearer), http.StatusOK)
		if len(mine.Items) != 1 || mine.Items[0].ID != file.ID {
			t.Fatalf("mine = %#v", mine.Items)
		}
		theirs := decode[itemPage[CourseFile]](t, e.api.Get("/study/files?mine=true&q="+marker, other.bearer), http.StatusOK)
		if len(theirs.Items) != 0 {
			t.Fatalf("other mine = %#v", theirs.Items)
		}
	})

	t.Run("a search word under 3 characters is invalid input", func(t *testing.T) {
		expectError(t, e.api.Get("/study/files?q=ab", other.bearer), http.StatusBadRequest, "invalid_input")
	})
}

func TestFilesOfBlockedStudents(t *testing.T) {
	e := newTestEnv(t)

	t.Run("are left out of lists and not found directly, in both directions", func(t *testing.T) {
		author := e.student(t)
		viewer := e.student(t)
		course := e.course(t, author, nil)
		file := e.file(t, author, course.ID, "Final exam")
		e.block(t, author.id, viewer.id)

		list := decode[itemPage[CourseFile]](t, e.api.Get("/study/courses/"+course.ID.String()+"/files", viewer.bearer), http.StatusOK)
		if len(list.Items) != 0 {
			t.Fatalf("list = %#v", list.Items)
		}
		expectError(t, e.api.Get("/study/files/"+file.ID.String(), viewer.bearer), http.StatusNotFound, "not_found")
		own := decode[itemPage[CourseFile]](t, e.api.Get("/study/courses/"+course.ID.String()+"/files", author.bearer), http.StatusOK)
		if len(own.Items) != 1 {
			t.Fatalf("author list = %#v", own.Items)
		}
	})
}

func TestDeleteFile(t *testing.T) {
	e := newTestEnv(t)

	t.Run("the author deletes the file and its upload", func(t *testing.T) {
		author := e.student(t)
		course := e.course(t, author, nil)
		file := e.file(t, author, course.ID, "Notes week 1")
		var mediaID uuid.UUID
		if err := e.pool.QueryRow(context.Background(), "SELECT media_id FROM course_files WHERE id = $1", file.ID).Scan(&mediaID); err != nil {
			t.Fatal(err)
		}

		if rec := e.api.Delete("/study/files/"+file.ID.String(), author.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		expectError(t, e.api.Get("/study/files/"+file.ID.String(), author.bearer), http.StatusNotFound, "not_found")
		var left bool
		if err := e.pool.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM media WHERE id = $1)", mediaID).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left {
			t.Error("the upload is still stored")
		}
		if rec := e.api.Delete("/study/files/"+file.ID.String(), author.bearer); rec.Code != http.StatusNoContent {
			t.Errorf("deleting again: expected 204, got %d", rec.Code)
		}
	})

	t.Run("another student's delete leaves the file", func(t *testing.T) {
		author := e.student(t)
		other := e.student(t)
		course := e.course(t, author, nil)
		file := e.file(t, author, course.ID, "Lab report")
		if rec := e.api.Delete("/study/files/"+file.ID.String(), other.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		decode[CourseFile](t, e.api.Get("/study/files/"+file.ID.String(), author.bearer), http.StatusOK)
	})
}
