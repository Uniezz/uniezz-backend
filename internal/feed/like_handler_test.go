package feed

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestPostLikes(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	liker := e.user(t)
	post := e.createPost(t, author, nil)
	path := "/feed/posts/" + post.ID.String() + "/like"

	steps := []struct {
		name  string
		do    func(string, ...any) *httptest.ResponseRecorder
		want  LikeState
		liked bool
	}{
		{"like", e.api.Put, LikeState{Liked: true, LikesCount: 1}, true},
		{"like again changes nothing", e.api.Put, LikeState{Liked: true, LikesCount: 1}, true},
		{"unlike", e.api.Delete, LikeState{Liked: false, LikesCount: 0}, false},
		{"unlike again changes nothing", e.api.Delete, LikeState{Liked: false, LikesCount: 0}, false},
	}
	for _, step := range steps {
		got := decode[LikeState](t, step.do(path, liker.bearer), 200)
		if got != step.want {
			t.Errorf("%s: got %+v, want %+v", step.name, got, step.want)
		}
		if p := decode[Post](t, e.api.Get("/feed/posts/"+post.ID.String(), liker.bearer), 200); p.LikedByMe != step.liked {
			t.Errorf("%s: likedByMe = %v", step.name, p.LikedByMe)
		}
	}

	expectError(t, e.api.Put("/feed/posts/"+uuid.NewString()+"/like", liker.bearer), 404, "not_found")
	e.assertCounters(t, post.ID)
}

// holdLock opens a transaction that locks the row with query, returns once
// the lock is held, and releases it when the returned function is called.
func (e *testEnv) holdLock(t *testing.T, query string, args ...any) func() {
	t.Helper()
	ctx := context.Background()

	tx, err := e.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	var once sync.Once
	release := func() { once.Do(func() { _ = tx.Commit(ctx) }) }
	t.Cleanup(release)
	return release
}

func TestConcurrentPostLikes(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)

	t.Run("one user double-tapping likes once", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		liker := e.user(t)

		release := e.holdLock(t, "SELECT 1 FROM feed_posts WHERE id = $1 FOR UPDATE", post.ID)
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				checkStatus(t, e.api.Put("/feed/posts/"+post.ID.String()+"/like", liker.bearer), 200)
			}()
		}
		waitForLockWait(t, 20, "FROM feed_posts WHERE id = $1 FOR NO KEY UPDATE")
		release()
		wg.Wait()

		if got := decode[Post](t, e.api.Get("/feed/posts/"+post.ID.String(), liker.bearer), 200); got.LikesCount != 1 {
			t.Errorf("likesCount = %d, want 1", got.LikesCount)
		}
		e.assertCounters(t, post.ID)
	})

	t.Run("many users and interleaved unlikes stay consistent", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		users := make([]testUser, 10)
		for i := range users {
			users[i] = e.user(t)
		}

		release := e.holdLock(t, "SELECT 1 FROM feed_posts WHERE id = $1 FOR UPDATE", post.ID)
		var wg sync.WaitGroup
		for i, u := range users {
			for j := range 4 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					path := "/feed/posts/" + post.ID.String() + "/like"
					if (i+j)%2 == 0 {
						checkStatus(t, e.api.Put(path, u.bearer), 200)
					} else {
						checkStatus(t, e.api.Delete(path, u.bearer), 200)
					}
				}()
			}
		}
		waitForLockWait(t, 20, "FROM feed_posts WHERE id = $1 FOR NO KEY UPDATE")
		release()
		wg.Wait()
		e.assertCounters(t, post.ID)
	})

	t.Run("likes racing a post delete", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		liker := e.user(t)

		release := e.holdLock(t, "SELECT 1 FROM feed_posts WHERE id = $1 FOR UPDATE", post.ID)
		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := e.api.Put("/feed/posts/"+post.ID.String()+"/like", liker.bearer)
				if rec.Code != 200 && rec.Code != 404 {
					t.Errorf("like status %d: %s", rec.Code, rec.Body.String())
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			checkStatus(t, e.api.Delete("/feed/posts/"+post.ID.String(), author.bearer), 204)
		}()
		waitForLockWait(t, 6, "FROM feed_posts WHERE id = $1 FOR")
		release()
		wg.Wait()

		var likes int
		_ = e.pool.QueryRow(context.Background(), "SELECT count(*) FROM feed_post_likes WHERE post_id = $1", post.ID).Scan(&likes)
		if likes != 0 {
			t.Errorf("%d likes outlived the post", likes)
		}
	})
}
