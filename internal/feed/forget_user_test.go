package feed

import (
	"context"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

func TestForgetUser(t *testing.T) {
	e := newTestEnv(t)
	leaving := e.user(t)
	staying := e.user(t)
	ctx := context.Background()

	ownMedia := e.insertMedia(t, leaving.id, media.KindFeed)
	ownPost := e.createPost(t, leaving, map[string]any{"mediaIds": []uuid.UUID{ownMedia}})
	e.comment(t, staying, ownPost.ID, nil, "On the leaving user's post")

	otherPost := e.createPost(t, staying, nil)
	answered := e.comment(t, leaving, otherPost.ID, nil, "Answered root")
	e.comment(t, staying, otherPost.ID, &answered.ID, "Answer")
	expectStatus(t, e.api.Put("/feed/comments/"+answered.ID.String()+"/like", staying.bearer), 200)
	lonely := e.comment(t, leaving, otherPost.ID, nil, "Nobody answered")
	stayingRoot := e.comment(t, staying, otherPost.ID, nil, "Staying root")
	e.comment(t, leaving, otherPost.ID, &stayingRoot.ID, "Leaving reply")
	ownThread := e.comment(t, leaving, otherPost.ID, nil, "Own thread")
	e.comment(t, leaving, otherPost.ID, &ownThread.ID, "Talking to myself")

	expectStatus(t, e.api.Put("/feed/posts/"+otherPost.ID.String()+"/like", leaving.bearer), 200)
	expectStatus(t, e.api.Put("/feed/comments/"+stayingRoot.ID.String()+"/like", leaving.bearer), 200)
	expectStatus(t, e.api.Post("/feed/posts", leaving.bearer, "Idempotency-Key: "+uuid.NewString(), postBody(nil)), 201)

	if err := e.module.ForgetUser(ctx, leaving.id); err != nil {
		t.Fatalf("ForgetUser: %v", err)
	}
	if err := e.module.ForgetUser(ctx, leaving.id); err != nil {
		t.Fatalf("second ForgetUser must be a no-op: %v", err)
	}

	var left int
	err := e.pool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM feed_posts WHERE author_id = $1)
		     + (SELECT count(*) FROM feed_comments WHERE author_id = $1)
		     + (SELECT count(*) FROM feed_post_likes WHERE user_id = $1)
		     + (SELECT count(*) FROM feed_comment_likes WHERE user_id = $1)`, leaving.id).Scan(&left)
	if err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d rows still reference the user", left)
	}
	if got := e.mediaStatus(t, ownMedia); got != "gone" {
		t.Errorf("post media = %q, want deleted", got)
	}

	page := decode[commentPage](t, e.api.Get("/feed/posts/"+otherPost.ID.String()+"/comments", staying.bearer), 200)
	var roots []string
	for _, c := range page.Items {
		switch {
		case c.ID == answered.ID && c.Deleted && c.RepliesCount == 1:
			roots = append(roots, "tombstone")
		case c.ID == stayingRoot.ID && c.RepliesCount == 0 && c.LikesCount == 0:
			roots = append(roots, "staying")
		default:
			t.Errorf("unexpected root %+v", c)
		}
	}
	if len(roots) != 2 {
		t.Errorf("roots = %v, want the answered tombstone and the staying root (lonely %s and own thread gone)", roots, lonely.ID)
	}
	if p := decode[Post](t, e.api.Get("/feed/posts/"+otherPost.ID.String(), staying.bearer), 200); p.LikesCount != 0 || p.CommentsCount != 2 {
		t.Errorf("post counters = likes %d comments %d, want 0 and 2", p.LikesCount, p.CommentsCount)
	}
	e.assertCounters(t, otherPost.ID)

	if _, err := e.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", leaving.id); err != nil {
		t.Errorf("deleting the user after ForgetUser: %v", err)
	}
}

func TestForgetUserRefusesATransaction(t *testing.T) {
	e := newTestEnv(t)
	u := e.user(t)
	err := e.module.tx.WithTx(context.Background(), func(ctx context.Context) error {
		return e.module.ForgetUser(ctx, u.id)
	})
	if err == nil {
		t.Fatal("ForgetUser inside a transaction must fail")
	}
}

func TestUserDeleteIsRefusedWhileFeedContentRemains(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	e.createPost(t, author, nil)

	if _, err := e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", author.id); err == nil {
		t.Fatal("deleting a user with posts must fail until ForgetUser ran")
	}
}

func TestPruneKeys(t *testing.T) {
	e := newTestEnv(t)
	u := e.user(t)
	ctx := context.Background()

	_, err := e.pool.Exec(ctx, `
		INSERT INTO feed_request_keys (user_id, operation, key, fingerprint, resource_id, created_at) VALUES
		($1, 'create_post', 'old', '\x00', gen_random_uuid(), NOW() - INTERVAL '25 hours'),
		($1, 'create_post', 'fresh', '\x00', gen_random_uuid(), NOW() - INTERVAL '23 hours')`, u.id)
	if err != nil {
		t.Fatal(err)
	}

	if err := e.module.pruneKeys(ctx, PruneKeysArgs{}); err != nil {
		t.Fatal(err)
	}

	var keys []string
	rows, _ := e.pool.Query(ctx, "SELECT key FROM feed_request_keys WHERE user_id = $1 ORDER BY key", u.id)
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		keys = append(keys, k)
	}
	if len(keys) != 1 || keys[0] != "fresh" {
		t.Errorf("keys = %v, want only the fresh one", keys)
	}
}
