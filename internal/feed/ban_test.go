package feed

import (
	"context"
	"net/http"
	"testing"
)

func TestBannedStudentsContentIsHidden(t *testing.T) {
	e := newTestEnv(t)
	author, viewer, bystander := e.user(t), e.user(t), e.user(t)

	post := e.createPost(t, author, nil)
	path := "/feed/posts/" + post.ID.String()
	other := e.createPost(t, bystander, nil)
	otherPath := "/feed/posts/" + other.ID.String()
	byAuthor := decode[Comment](t, e.api.Post(otherPath+"/comments", author.bearer, map[string]any{"content": "hidden"}), http.StatusCreated)
	visible := decode[Comment](t, e.api.Post(otherPath+"/comments", bystander.bearer, map[string]any{"content": "shown"}), http.StatusCreated)

	if _, err := e.pool.Exec(context.Background(), "UPDATE users SET banned_at = NOW() WHERE id = $1", author.id); err != nil {
		t.Fatal(err)
	}

	t.Run("posts are hidden from lists and not found directly", func(t *testing.T) {
		expectError(t, e.api.Get(path, viewer.bearer), http.StatusNotFound, "not_found")
		if page := decode[postPage](t, e.api.Get("/feed/posts?authorId="+author.id.String(), viewer.bearer), http.StatusOK); len(page.Items) != 0 {
			t.Errorf("posts = %+v", page.Items)
		}
	})

	t.Run("comments are hidden on other students' posts", func(t *testing.T) {
		page := decode[commentPage](t, e.api.Get(otherPath+"/comments", viewer.bearer), http.StatusOK)
		if len(page.Items) != 1 || page.Items[0].ID != visible.ID {
			t.Errorf("comments = %+v", page.Items)
		}
	})

	t.Run("hidden posts and comments cannot be liked or answered", func(t *testing.T) {
		expectError(t, e.api.Put(path+"/like", viewer.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Post(path+"/comments", viewer.bearer, map[string]any{"content": "x"}), http.StatusNotFound, "not_found")
		expectError(t, e.api.Put("/feed/comments/"+byAuthor.ID.String()+"/like", viewer.bearer), http.StatusNotFound, "not_found")
	})
}
