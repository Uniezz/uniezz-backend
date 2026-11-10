package feed

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSavedAndLikedPosts(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	viewer := e.user(t)
	other := e.user(t)
	ctx := context.Background()

	first := e.createPost(t, author, nil)
	second := e.createPost(t, author, nil)

	expectError(t, e.api.Put("/feed/posts/"+uuid.NewString()+"/save", viewer.bearer), http.StatusNotFound, "not_found")

	save := "/feed/posts/" + first.ID.String() + "/save"
	for _, step := range []string{"save", "save again"} {
		got := decode[saveState](t, e.api.Put(save, viewer.bearer), http.StatusOK)
		if !got.Saved {
			t.Errorf("%s: saved = false", step)
		}
	}
	if post := decode[Post](t, e.api.Get("/feed/posts/"+first.ID.String(), viewer.bearer), http.StatusOK); !post.SavedByMe || post.LikedByMe {
		t.Errorf("after save: savedByMe %v likedByMe %v", post.SavedByMe, post.LikedByMe)
	}

	saved := decode[postPage](t, e.api.Get("/feed/posts/saved", viewer.bearer), http.StatusOK)
	if len(saved.Items) != 1 || saved.Items[0].ID != first.ID || !saved.Items[0].SavedByMe {
		t.Fatalf("saved = %+v", saved.Items)
	}
	if otherSaved := decode[postPage](t, e.api.Get("/feed/posts/saved", other.bearer), http.StatusOK); otherSaved.Items == nil || len(otherSaved.Items) != 0 {
		t.Errorf("another student sees saves: %+v", otherSaved.Items)
	}

	for _, step := range []string{"unsave", "unsave again"} {
		got := decode[saveState](t, e.api.Delete(save, viewer.bearer), http.StatusOK)
		if got.Saved {
			t.Errorf("%s: saved = true", step)
		}
	}
	if post := decode[Post](t, e.api.Get("/feed/posts/"+first.ID.String(), viewer.bearer), http.StatusOK); post.SavedByMe {
		t.Error("savedByMe stayed true after unsave")
	}

	expectStatus(t, e.api.Put("/feed/posts/"+first.ID.String()+"/like", viewer.bearer), http.StatusOK)
	expectStatus(t, e.api.Put("/feed/posts/"+second.ID.String()+"/like", viewer.bearer), http.StatusOK)
	expectStatus(t, e.api.Put("/feed/posts/"+first.ID.String()+"/save", viewer.bearer), http.StatusOK)
	expectStatus(t, e.api.Put("/feed/posts/"+second.ID.String()+"/save", viewer.bearer), http.StatusOK)

	older := time.Now().Add(-time.Hour)
	newer := older.Add(time.Minute)
	setSavedAt := func(postID uuid.UUID, at time.Time) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, "UPDATE feed_post_saves SET created_at = $3 WHERE post_id = $1 AND user_id = $2", postID, viewer.id, at); err != nil {
			t.Fatal(err)
		}
	}
	setSavedAt(first.ID, newer)
	setSavedAt(second.ID, older)

	checkPages := func(path string, want []uuid.UUID) {
		t.Helper()
		page := decode[postPage](t, e.api.Get(path+"?limit=1", viewer.bearer), http.StatusOK)
		if page.NextCursor == nil || len(page.Items) != 1 || page.Items[0].ID != want[0] {
			t.Fatalf("%s first page = %+v cursor %v", path, page.Items, page.NextCursor)
		}
		next := decode[postPage](t, e.api.Get(path+"?limit=1&cursor="+*page.NextCursor, viewer.bearer), http.StatusOK)
		if next.NextCursor != nil || len(next.Items) != 1 || next.Items[0].ID != want[1] {
			t.Fatalf("%s second page = %+v cursor %v", path, next.Items, next.NextCursor)
		}
	}
	checkPages("/feed/posts/liked", []uuid.UUID{second.ID, first.ID})
	checkPages("/feed/posts/saved", []uuid.UUID{first.ID, second.ID})

	expectStatus(t, e.api.Delete("/feed/posts/"+second.ID.String()+"/like", viewer.bearer), http.StatusOK)
	liked := decode[postPage](t, e.api.Get("/feed/posts/liked", viewer.bearer), http.StatusOK)
	if len(liked.Items) != 1 || liked.Items[0].ID != first.ID || !liked.Items[0].LikedByMe {
		t.Fatalf("liked after unlike = %+v", liked.Items)
	}

	expectStatus(t, e.api.Delete("/feed/posts/"+first.ID.String(), author.bearer), http.StatusNoContent)
	for _, path := range []string{"/feed/posts/saved", "/feed/posts/liked"} {
		page := decode[postPage](t, e.api.Get(path, viewer.bearer), http.StatusOK)
		for _, item := range page.Items {
			if item.ID == first.ID {
				t.Errorf("%s still lists deleted post", path)
			}
		}
	}
}
