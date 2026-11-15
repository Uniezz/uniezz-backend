package feed

import (
	"context"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/rewards"
)

func (e *testEnv) coins(t *testing.T, u testUser) int {
	t.Helper()
	m, err := rewards.New(e.pool, jobs.NewRunner(e.pool, 1, 0))
	if err != nil {
		t.Fatalf("rewards.New: %v", err)
	}
	n, err := m.Balance(context.Background(), u.id)
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

func (e *testEnv) quests(t *testing.T, u testUser) (likes int, replied, posted bool) {
	t.Helper()
	err := e.pool.QueryRow(context.Background(), `
		SELECT COALESCE(sum(likes), 0), COALESCE(bool_or(replied), FALSE), COALESCE(bool_or(posted), FALSE)
		FROM daily_quests WHERE user_id = $1`, u.id).Scan(&likes, &replied, &posted)
	if err != nil {
		t.Fatalf("read quests: %v", err)
	}
	return likes, replied, posted
}

func TestDailyQuests(t *testing.T) {
	e := newTestEnv(t)
	like := func(t *testing.T, u testUser, post Post) {
		t.Helper()
		expectStatus(t, e.api.Put("/feed/posts/"+post.ID.String()+"/like", u.bearer), http.StatusOK)
	}

	t.Run("three likes, a reply and a post pay 10 coins on top", func(t *testing.T) {
		u, other := e.user(t), e.user(t)
		for range 3 {
			like(t, u, e.createPost(t, other, nil))
		}
		post := e.createPost(t, other, nil)
		root := e.comment(t, other, post.ID, nil, "Question")
		e.comment(t, u, post.ID, &root.ID, "Answer")
		e.createPost(t, u, nil)

		if got := e.coins(t, u); got != 3+1+10 {
			t.Errorf("coins = %d, want 14 (3 likes, 1 comment, 10 quests)", got)
		}
	})

	t.Run("unliking and liking the same post again counts one like", func(t *testing.T) {
		u, other := e.user(t), e.user(t)
		post := e.createPost(t, other, nil)
		like(t, u, post)
		expectStatus(t, e.api.Delete("/feed/posts/"+post.ID.String()+"/like", u.bearer), http.StatusOK)
		like(t, u, post)
		like(t, u, e.createPost(t, u, nil))
		if likes, _, _ := e.quests(t, u); likes != 1 {
			t.Errorf("likes = %d, want 1", likes)
		}
	})

	t.Run("a root comment or a reply to yourself is not a reply quest", func(t *testing.T) {
		u, other := e.user(t), e.user(t)
		post := e.createPost(t, other, nil)
		mine := e.comment(t, u, post.ID, nil, "Root")
		e.comment(t, u, post.ID, &mine.ID, "Self reply")
		if _, replied, _ := e.quests(t, u); replied {
			t.Error("replied = true, want false")
		}
	})

	t.Run("a reply to another student's comment under your own post counts", func(t *testing.T) {
		u, other := e.user(t), e.user(t)
		post := e.createPost(t, u, nil)
		theirs := e.comment(t, other, post.ID, nil, "Nice post")
		e.comment(t, u, post.ID, &theirs.ID, "Thanks")
		if _, replied, posted := e.quests(t, u); !replied || !posted {
			t.Errorf("replied %v, posted %v; want true, true", replied, posted)
		}
	})
}

func TestCoinsEarned(t *testing.T) {
	e := newTestEnv(t)
	type earned struct {
		CoinsEarned int `json:"coinsEarned"`
	}
	likePost := func(t *testing.T, u testUser, post Post) int {
		t.Helper()
		return decode[earned](t, e.api.Put("/feed/posts/"+post.ID.String()+"/like", u.bearer), http.StatusOK).CoinsEarned
	}

	t.Run("the first like returns 1 and liking again returns 0", func(t *testing.T) {
		u, other := e.user(t), e.user(t)
		post := e.createPost(t, other, nil)
		if got := likePost(t, u, post); got != 1 {
			t.Errorf("first like = %d, want 1", got)
		}
		if got := likePost(t, u, post); got != 0 {
			t.Errorf("like again = %d, want 0", got)
		}
	})

	t.Run("the action that finishes the daily quests returns the quest reward too", func(t *testing.T) {
		u, other := e.user(t), e.user(t)
		post := e.createPost(t, other, nil)
		root := e.comment(t, other, post.ID, nil, "Question")
		reply := decode[earned](t, e.api.Post("/feed/posts/"+post.ID.String()+"/comments", u.bearer,
			map[string]any{"content": "Answer", "replyToCommentId": root.ID}), http.StatusCreated)
		if reply.CoinsEarned != 1 {
			t.Errorf("reply = %d, want 1", reply.CoinsEarned)
		}
		own := decode[earned](t, e.api.Post("/feed/posts", u.bearer, postBody(nil)), http.StatusCreated)
		if own.CoinsEarned != 0 {
			t.Errorf("post = %d, want 0", own.CoinsEarned)
		}
		likePost(t, u, e.createPost(t, other, nil))
		likePost(t, u, e.createPost(t, other, nil))
		if got := likePost(t, u, e.createPost(t, other, nil)); got != 11 {
			t.Errorf("third like = %d, want 11 (1 for the like, 10 for the quests)", got)
		}
	})

	t.Run("a comment on your own post returns 0", func(t *testing.T) {
		u := e.user(t)
		post := e.createPost(t, u, nil)
		got := decode[earned](t, e.api.Post("/feed/posts/"+post.ID.String()+"/comments", u.bearer, map[string]any{"content": "Mine"}), http.StatusCreated)
		if got.CoinsEarned != 0 {
			t.Errorf("coinsEarned = %d, want 0", got.CoinsEarned)
		}
	})
}
