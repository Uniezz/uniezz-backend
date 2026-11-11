package feed

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) block(t *testing.T, blocker, blocked testUser) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), "INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)", blocker.id, blocked.id)
	if err != nil {
		t.Fatalf("block: %v", err)
	}
}

func TestBlockedStudentsDoNotSeeEachOther(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	viewer := e.user(t)
	bystander := e.user(t)

	post := e.createPost(t, author, nil)
	path := "/feed/posts/" + post.ID.String()
	root := decode[Comment](t, e.api.Post(path+"/comments", bystander.bearer, map[string]any{"content": "first"}), http.StatusCreated)
	byAuthor := decode[Comment](t, e.api.Post(path+"/comments", author.bearer, map[string]any{"content": "second"}), http.StatusCreated)
	expectStatus(t, e.api.Put(path+"/save", viewer.bearer), http.StatusOK)
	expectStatus(t, e.api.Put(path+"/like", viewer.bearer), http.StatusOK)

	e.block(t, viewer, author)

	t.Run("posts by the other student are hidden", func(t *testing.T) {
		expectError(t, e.api.Get(path, viewer.bearer), http.StatusNotFound, "not_found")
		for _, list := range []string{"/feed/posts?authorId=" + author.id.String(), "/feed/posts/saved", "/feed/posts/liked"} {
			if page := decode[postPage](t, e.api.Get(list, viewer.bearer), http.StatusOK); len(page.Items) != 0 {
				t.Errorf("%s = %+v", list, page.Items)
			}
		}
		expectError(t, e.api.Get(path+"/comments", viewer.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("the block works both ways", func(t *testing.T) {
		own := e.createPost(t, viewer, nil)
		expectError(t, e.api.Get("/feed/posts/"+own.ID.String(), author.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("comments by the other student are hidden on other posts", func(t *testing.T) {
		other := e.createPost(t, bystander, nil)
		otherPath := "/feed/posts/" + other.ID.String()
		decode[Comment](t, e.api.Post(otherPath+"/comments", author.bearer, map[string]any{"content": "hidden"}), http.StatusCreated)
		visible := decode[Comment](t, e.api.Post(otherPath+"/comments", bystander.bearer, map[string]any{"content": "shown"}), http.StatusCreated)

		page := decode[commentPage](t, e.api.Get(otherPath+"/comments", viewer.bearer), http.StatusOK)
		if len(page.Items) != 1 || page.Items[0].ID != visible.ID {
			t.Errorf("comments = %+v", page.Items)
		}
		if page := decode[commentPage](t, e.api.Get(otherPath+"/comments", bystander.bearer), http.StatusOK); len(page.Items) != 2 {
			t.Errorf("a student outside the block sees %d comments, want 2", len(page.Items))
		}
	})

	t.Run("hidden posts and comments cannot be liked, saved or answered", func(t *testing.T) {
		expectError(t, e.api.Put(path+"/like", viewer.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Put(path+"/save", viewer.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Post(path+"/comments", viewer.bearer, map[string]any{"content": "x"}), http.StatusNotFound, "not_found")
		expectError(t, e.api.Put("/feed/comments/"+byAuthor.ID.String()+"/like", viewer.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Get("/feed/comments/"+root.ID.String()+"/replies", viewer.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("undoing a like or save on a hidden post still works", func(t *testing.T) {
		expectStatus(t, e.api.Delete(path+"/like", viewer.bearer), http.StatusOK)
		expectStatus(t, e.api.Delete(path+"/save", viewer.bearer), http.StatusOK)
	})

	t.Run("an unknown post is still a 404", func(t *testing.T) {
		expectError(t, e.api.Get("/feed/posts/"+uuid.NewString(), viewer.bearer), http.StatusNotFound, "not_found")
	})
}
