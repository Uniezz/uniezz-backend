package feed

import (
	"context"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/rewards"
)

func (e *testEnv) coins(t *testing.T, u testUser) int {
	t.Helper()
	n, err := rewards.New(e.pool).Balance(context.Background(), u.id)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	return n
}

func TestLikeCoins(t *testing.T) {
	e := newTestEnv(t)
	like := func(t *testing.T, u testUser, post Post) {
		t.Helper()
		expectStatus(t, e.api.Put("/feed/posts/"+post.ID.String()+"/like", u.bearer), http.StatusOK)
	}

	t.Run("a like pays the liker and the author one coin each", func(t *testing.T) {
		author, liker := e.user(t), e.user(t)
		like(t, liker, e.createPost(t, author, nil))
		if a, l := e.coins(t, author), e.coins(t, liker); a != 1 || l != 1 {
			t.Errorf("author %d, liker %d; want 1, 1", a, l)
		}
	})

	t.Run("unliking and liking again pays nothing more", func(t *testing.T) {
		author, liker := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		like(t, liker, post)
		expectStatus(t, e.api.Delete("/feed/posts/"+post.ID.String()+"/like", liker.bearer), http.StatusOK)
		like(t, liker, post)
		like(t, liker, post)
		if a, l := e.coins(t, author), e.coins(t, liker); a != 1 || l != 1 {
			t.Errorf("author %d, liker %d; want 1, 1", a, l)
		}
	})

	t.Run("each liker pays the author once and the liker earns once per post", func(t *testing.T) {
		author, first, second := e.user(t), e.user(t), e.user(t)
		post, other := e.createPost(t, author, nil), e.createPost(t, author, nil)
		like(t, first, post)
		like(t, second, post)
		like(t, first, other)
		if a, f, s := e.coins(t, author), e.coins(t, first), e.coins(t, second); a != 3 || f != 2 || s != 1 {
			t.Errorf("author %d, first %d, second %d; want 3, 2, 1", a, f, s)
		}
	})

	t.Run("liking your own post pays nothing", func(t *testing.T) {
		author := e.user(t)
		like(t, author, e.createPost(t, author, nil))
		if got := e.coins(t, author); got != 0 {
			t.Errorf("coins = %d, want 0", got)
		}
	})

	t.Run("liking a comment pays nothing", func(t *testing.T) {
		author, liker := e.user(t), e.user(t)
		c := e.comment(t, author, e.createPost(t, author, nil).ID, nil, "Hi")
		expectStatus(t, e.api.Put("/feed/comments/"+c.ID.String()+"/like", liker.bearer), http.StatusOK)
		if a, l := e.coins(t, author), e.coins(t, liker); a != 0 || l != 0 {
			t.Errorf("author %d, liker %d; want 0, 0", a, l)
		}
	})
}

func TestCommentCoins(t *testing.T) {
	e := newTestEnv(t)

	t.Run("comments and replies on one post pay the commenter once", func(t *testing.T) {
		author, commenter := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		first := e.comment(t, commenter, post.ID, nil, "First")
		e.comment(t, commenter, post.ID, &first.ID, "Reply")
		e.comment(t, commenter, post.ID, nil, "Second")
		if got := e.coins(t, commenter); got != 1 {
			t.Errorf("commenter coins = %d, want 1", got)
		}
		if got := e.coins(t, author); got != 0 {
			t.Errorf("author coins = %d, want 0", got)
		}
	})

	t.Run("a comment on another post pays again", func(t *testing.T) {
		author, commenter := e.user(t), e.user(t)
		e.comment(t, commenter, e.createPost(t, author, nil).ID, nil, "One")
		e.comment(t, commenter, e.createPost(t, author, nil).ID, nil, "Two")
		if got := e.coins(t, commenter); got != 2 {
			t.Errorf("coins = %d, want 2", got)
		}
	})

	t.Run("deleting a comment and writing another pays nothing more", func(t *testing.T) {
		author, commenter := e.user(t), e.user(t)
		post := e.createPost(t, author, nil)
		c := e.comment(t, commenter, post.ID, nil, "Gone")
		expectStatus(t, e.api.Delete("/feed/comments/"+c.ID.String(), commenter.bearer), http.StatusNoContent)
		e.comment(t, commenter, post.ID, nil, "Again")
		if got := e.coins(t, commenter); got != 1 {
			t.Errorf("coins = %d, want 1", got)
		}
	})

	t.Run("commenting on your own post pays nothing", func(t *testing.T) {
		author := e.user(t)
		e.comment(t, author, e.createPost(t, author, nil).ID, nil, "Mine")
		if got := e.coins(t, author); got != 0 {
			t.Errorf("coins = %d, want 0", got)
		}
	})
}
