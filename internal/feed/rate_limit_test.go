package feed

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) postCount(t *testing.T, authorID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM feed_posts WHERE author_id = $1", authorID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *testEnv) commentCount(t *testing.T, authorID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM feed_comments WHERE author_id = $1", authorID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPostRateLimit(t *testing.T) {
	e := newRateLimitedTestEnv(t)

	t.Run("a second post right after the first is rate limited and not stored", func(t *testing.T) {
		u := e.user(t)
		e.createPost(t, u, nil)

		expectError(t, e.api.Post("/feed/posts", u.bearer, postBody(nil)), http.StatusTooManyRequests, "rate_limited")
		if n := e.postCount(t, u.id); n != 1 {
			t.Errorf("expected 1 post, got %d", n)
		}
	})

	t.Run("one student's limit does not affect another", func(t *testing.T) {
		a, b := e.user(t), e.user(t)
		e.createPost(t, a, nil)
		e.createPost(t, b, nil)
	})

	t.Run("a retry with the same idempotency key replays without counting", func(t *testing.T) {
		u := e.user(t)
		key := "Idempotency-Key: " + uuid.NewString()
		first := decode[Post](t, e.api.Post("/feed/posts", u.bearer, key, postBody(nil)), http.StatusCreated)
		again := decode[Post](t, e.api.Post("/feed/posts", u.bearer, key, postBody(nil)), http.StatusCreated)
		if again.ID != first.ID {
			t.Errorf("expected replay of %s, got %s", first.ID, again.ID)
		}
	})

	t.Run("a rejected post costs no quota", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Post("/feed/posts", u.bearer, postBody(map[string]any{"title": ""})), http.StatusBadRequest, "invalid_input")
		e.createPost(t, u, nil)
	})

	t.Run("concurrent posts from one student store exactly one", func(t *testing.T) {
		u := e.user(t)
		const n = 5
		codes := make([]int, n)
		var wg sync.WaitGroup
		for i := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes[i] = e.api.Post("/feed/posts", u.bearer, postBody(nil)).Code
			}()
		}
		wg.Wait()

		created := 0
		for _, c := range codes {
			switch c {
			case http.StatusCreated:
				created++
			case http.StatusTooManyRequests:
			default:
				t.Errorf("unexpected status %d", c)
			}
		}
		if created != 1 {
			t.Errorf("expected 1 created, got %d", created)
		}
		if got := e.postCount(t, u.id); got != 1 {
			t.Errorf("expected 1 stored post, got %d", got)
		}
	})

	t.Run("without a session returns 401", func(t *testing.T) {
		expectStatus(t, e.api.Post("/feed/posts", postBody(nil)), http.StatusUnauthorized)
	})
}

func TestCommentRateLimit(t *testing.T) {
	e := newRateLimitedTestEnv(t)

	t.Run("the sixth comment in a minute is rate limited and not stored", func(t *testing.T) {
		author, u := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		for range commentBurstLimit {
			e.comment(t, u, post.ID, nil, "Nice")
		}

		expectError(t, e.api.Post("/feed/posts/"+post.ID.String()+"/comments", u.bearer, map[string]any{"content": "Nice"}), http.StatusTooManyRequests, "rate_limited")
		if n := e.commentCount(t, u.id); n != commentBurstLimit {
			t.Errorf("expected %d comments, got %d", commentBurstLimit, n)
		}
	})

	t.Run("comments and posts are limited separately", func(t *testing.T) {
		u := e.user(t)
		post := e.createPost(t, u, nil)
		e.comment(t, u, post.ID, nil, "First")
	})
}
