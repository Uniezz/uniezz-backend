package feed

import (
	"context"
	"net/http"
	"testing"
)

func TestHideReported(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("a hidden post is not found by others and still shown to its author", func(t *testing.T) {
		author, viewer := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		path := "/feed/posts/" + post.ID.String()
		list := "/feed/posts?authorId=" + author.id.String()

		if err := e.module.HideReported(ctx, TargetPost, post.ID); err != nil {
			t.Fatal(err)
		}

		expectError(t, e.api.Get(path, viewer.bearer), http.StatusNotFound, "not_found")
		if page := decode[postPage](t, e.api.Get(list, viewer.bearer), http.StatusOK); len(page.Items) != 0 {
			t.Errorf("others see posts = %+v", page.Items)
		}
		expectError(t, e.api.Put(path+"/like", viewer.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Post(path+"/comments", viewer.bearer, map[string]any{"content": "x"}), http.StatusNotFound, "not_found")

		decode[Post](t, e.api.Get(path, author.bearer), http.StatusOK)
		if page := decode[postPage](t, e.api.Get(list, author.bearer), http.StatusOK); len(page.Items) != 1 {
			t.Errorf("author sees %d posts, want 1", len(page.Items))
		}
	})

	t.Run("a restored post is shown again and is not hidden a second time", func(t *testing.T) {
		author, viewer := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		path := "/feed/posts/" + post.ID.String()

		if err := e.module.HideReported(ctx, TargetPost, post.ID); err != nil {
			t.Fatal(err)
		}
		if err := e.module.RestoreReported(ctx, TargetPost, post.ID); err != nil {
			t.Fatal(err)
		}
		decode[Post](t, e.api.Get(path, viewer.bearer), http.StatusOK)

		if err := e.module.HideReported(ctx, TargetPost, post.ID); err != nil {
			t.Fatal(err)
		}
		decode[Post](t, e.api.Get(path, viewer.bearer), http.StatusOK)
	})

	t.Run("editing a restored post lets reports hide it again", func(t *testing.T) {
		author, viewer := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		path := "/feed/posts/" + post.ID.String()

		if err := e.module.RestoreReported(ctx, TargetPost, post.ID); err != nil {
			t.Fatal(err)
		}
		decode[Post](t, e.api.Patch(path, author.bearer, map[string]any{"title": "Changed after review"}), http.StatusOK)
		if err := e.module.HideReported(ctx, TargetPost, post.ID); err != nil {
			t.Fatal(err)
		}
		expectError(t, e.api.Get(path, viewer.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("a hidden comment is left out for others and shown to its author", func(t *testing.T) {
		postAuthor, commenter, viewer := e.user(t), e.user(t), e.user(t)
		post := e.createPost(t, postAuthor, nil)
		comments := "/feed/posts/" + post.ID.String() + "/comments"
		hidden := e.comment(t, commenter, post.ID, nil, "Spam")
		shown := e.comment(t, viewer, post.ID, nil, "Fine")

		if err := e.module.HideReported(ctx, TargetComment, hidden.ID); err != nil {
			t.Fatal(err)
		}

		page := decode[commentPage](t, e.api.Get(comments, viewer.bearer), http.StatusOK)
		if len(page.Items) != 1 || page.Items[0].ID != shown.ID {
			t.Errorf("others see comments = %+v", page.Items)
		}
		expectError(t, e.api.Put("/feed/comments/"+hidden.ID.String()+"/like", viewer.bearer), http.StatusNotFound, "not_found")
		if page := decode[commentPage](t, e.api.Get(comments, commenter.bearer), http.StatusOK); len(page.Items) != 2 {
			t.Errorf("author sees %d comments, want 2", len(page.Items))
		}

		if err := e.module.RestoreReported(ctx, TargetComment, hidden.ID); err != nil {
			t.Fatal(err)
		}
		if page := decode[commentPage](t, e.api.Get(comments, viewer.bearer), http.StatusOK); len(page.Items) != 2 {
			t.Errorf("after restore others see %d comments, want 2", len(page.Items))
		}
	})

	t.Run("hiding or restoring a deleted post changes nothing", func(t *testing.T) {
		author := e.user(t)
		post := e.createPost(t, author, nil)
		expectStatus(t, e.api.Delete("/feed/posts/"+post.ID.String(), author.bearer), http.StatusNoContent)

		if err := e.module.HideReported(ctx, TargetPost, post.ID); err != nil {
			t.Errorf("hide: %v", err)
		}
		if err := e.module.RestoreReported(ctx, TargetPost, post.ID); err != nil {
			t.Errorf("restore: %v", err)
		}
	})
}
