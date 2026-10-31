package feed

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
)

type commentPage struct {
	Items      []Comment `json:"items"`
	NextCursor *string   `json:"nextCursor"`
}

func (e *testEnv) comment(t *testing.T, author testUser, postID uuid.UUID, replyTo *uuid.UUID, content string) Comment {
	t.Helper()
	body := map[string]any{"content": content}
	if replyTo != nil {
		body["replyToCommentId"] = *replyTo
	}
	return decode[Comment](t, e.api.Post("/feed/posts/"+postID.String()+"/comments", author.bearer, body), 201)
}

func (e *testEnv) commentsCount(t *testing.T, postID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT comments_count FROM feed_posts WHERE id = $1", postID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCreateComment(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	commenter := e.user(t)
	post := e.createPost(t, author, nil)

	root := e.comment(t, commenter, post.ID, nil, " <b>Great</b> post ")
	if *root.Content != "Great post" || root.ParentID != nil || root.ReplyToCommentID != nil || root.Author.ID != commenter.id {
		t.Errorf("root = %+v", root)
	}

	reply := e.comment(t, author, post.ID, &root.ID, "Thanks")
	if *reply.ParentID != root.ID || *reply.ReplyToCommentID != root.ID {
		t.Errorf("reply to root = %+v", reply)
	}

	nested := e.comment(t, commenter, post.ID, &reply.ID, "You're welcome")
	if *nested.ParentID != root.ID || *nested.ReplyToCommentID != reply.ID {
		t.Errorf("reply to a reply must join the root's thread, got %+v", nested)
	}

	if got := e.commentsCount(t, post.ID); got != 3 {
		t.Errorf("comments_count = %d, want 3", got)
	}
	e.assertCounters(t, post.ID)

	t.Run("rejects", func(t *testing.T) {
		other := e.createPost(t, author, nil)
		otherRoot := e.comment(t, commenter, other.ID, nil, "Elsewhere")

		path := "/feed/posts/" + post.ID.String() + "/comments"
		expectError(t, e.api.Post("/feed/posts/"+uuid.NewString()+"/comments", commenter.bearer, map[string]any{"content": "x"}), 404, "not_found")
		expectError(t, e.api.Post(path, commenter.bearer, map[string]any{"content": "x", "replyToCommentId": otherRoot.ID}), 404, "not_found")
		expectError(t, e.api.Post(path, commenter.bearer, map[string]any{"content": "x", "replyToCommentId": uuid.New()}), 404, "not_found")
		expectError(t, e.api.Post(path, commenter.bearer, map[string]any{"content": "<i></i>"}), 400, "invalid_input")
		expectError(t, e.api.Post(path, commenter.bearer, map[string]any{"content": strings.Repeat("ł", 1001)}), 400, "invalid_input")
		decode[Comment](t, e.api.Post(path, commenter.bearer, map[string]any{"content": strings.Repeat("ł", 1000)}), 201)
	})

	t.Run("database rejects a broken tree even without the service", func(t *testing.T) {
		other := e.createPost(t, author, nil)
		ctx := context.Background()
		for name, query := range map[string]string{
			"parent in another post":   `INSERT INTO feed_comments (post_id, author_id, parent_id, content) VALUES ($1, $2, $3, 'x')`,
			"parent is a reply":        `INSERT INTO feed_comments (post_id, author_id, parent_id, content) VALUES ($4, $2, $5, 'x')`,
			"reply target elsewhere":   `INSERT INTO feed_comments (post_id, author_id, parent_id, reply_to_comment_id, content) VALUES ($4, $2, $3, $6, 'x')`,
			"root with a reply target": `INSERT INTO feed_comments (post_id, author_id, reply_to_comment_id, content) VALUES ($4, $2, $3, 'x')`,
		} {
			otherRoot := e.comment(t, commenter, other.ID, nil, "Elsewhere")
			_, err := e.pool.Exec(ctx, query, other.ID, commenter.id, root.ID, post.ID, reply.ID, otherRoot.ID)
			if err == nil {
				t.Errorf("%s: insert succeeded", name)
			}
		}
	})
}

func TestCommentIdempotency(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	post := e.createPost(t, author, nil)
	other := e.createPost(t, author, nil)
	key := "Idempotency-Key: " + uuid.NewString()

	first := decode[Comment](t, e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, key, map[string]any{"content": "Hello"}), 201)
	rec := e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, key, map[string]any{"content": "Hello"})
	if got := decode[Comment](t, rec, 201); got.ID != first.ID || rec.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("replay = %+v", got)
	}
	expectError(t, e.api.Post("/feed/posts/"+other.ID.String()+"/comments", author.bearer, key, map[string]any{"content": "Hello"}), 422, "idempotency_key_reused")

	if got := e.commentsCount(t, post.ID); got != 1 {
		t.Errorf("comments_count = %d, want 1", got)
	}

	expectStatus(t, e.api.Delete("/feed/comments/"+first.ID.String(), author.bearer), 204)
	expectError(t, e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, key, map[string]any{"content": "Hello"}), 410, "resource_deleted")
}

func TestListComments(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	post := e.createPost(t, author, nil)

	var roots []uuid.UUID
	for range 5 {
		roots = append(roots, e.comment(t, author, post.ID, nil, "Root").ID)
	}
	_, err := e.pool.Exec(context.Background(), "UPDATE feed_comments SET created_at = '2026-01-01T00:00:00Z' WHERE post_id = $1", post.ID)
	if err != nil {
		t.Fatal(err)
	}
	var replies []uuid.UUID
	for range 3 {
		replies = append(replies, e.comment(t, author, post.ID, &roots[0], "Reply").ID)
	}

	walk := func(t *testing.T, path string) []uuid.UUID {
		t.Helper()
		var ids []uuid.UUID
		cursor := ""
		for range 10 {
			q := url.Values{"limit": {"2"}}
			if cursor != "" {
				q.Set("cursor", cursor)
			}
			page := decode[commentPage](t, e.api.Get(path+"?"+q.Encode(), author.bearer), 200)
			for _, c := range page.Items {
				ids = append(ids, c.ID)
			}
			if page.NextCursor == nil {
				return ids
			}
			cursor = *page.NextCursor
		}
		t.Fatal("pagination did not end")
		return nil
	}

	gotRoots := walk(t, "/feed/posts/"+post.ID.String()+"/comments")
	if len(gotRoots) != 5 {
		t.Fatalf("roots = %d, want 5", len(gotRoots))
	}
	for i := 1; i < len(gotRoots); i++ {
		if gotRoots[i-1].String() > gotRoots[i].String() {
			t.Errorf("equal timestamps must order by id ascending: %v", gotRoots)
		}
	}

	gotReplies := walk(t, "/feed/comments/"+roots[0].String()+"/replies")
	if len(gotReplies) != 3 || gotReplies[0] != replies[0] || gotReplies[2] != replies[2] {
		t.Errorf("replies = %v, want %v oldest first", gotReplies, replies)
	}

	expectError(t, e.api.Get("/feed/comments/"+replies[0].String()+"/replies", author.bearer), 404, "not_found")
	expectError(t, e.api.Get("/feed/posts/"+uuid.NewString()+"/comments", author.bearer), 404, "not_found")
	expectError(t, e.api.Get("/feed/posts/"+post.ID.String()+"/comments?cursor=bad", author.bearer), 400, "invalid_cursor")
}

func TestUpdateComment(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	post := e.createPost(t, author, nil)
	c := e.comment(t, author, post.ID, nil, "Original")
	path := "/feed/comments/" + c.ID.String()

	got := decode[Comment](t, e.api.Patch(path, author.bearer, map[string]any{"content": "Edited"}), 200)
	if *got.Content != "Edited" || got.EditedAt == nil {
		t.Errorf("edited = %+v", got)
	}
	expectError(t, e.api.Patch(path, e.user(t).bearer, map[string]any{"content": "Hijacked"}), 403, "forbidden")
	expectStatus(t, e.api.Delete(path, author.bearer), 204)
	expectError(t, e.api.Patch(path, author.bearer, map[string]any{"content": "Back"}), 404, "not_found")
}

func TestDeleteComment(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	commenter := e.user(t)
	post := e.createPost(t, author, nil)

	t.Run("only the author", func(t *testing.T) {
		c := e.comment(t, commenter, post.ID, nil, "Mine")
		expectError(t, e.api.Delete("/feed/comments/"+c.ID.String(), author.bearer), 403, "forbidden")
		expectStatus(t, e.api.Delete("/feed/comments/"+c.ID.String(), commenter.bearer), 204)
	})

	t.Run("root without replies disappears", func(t *testing.T) {
		before := e.commentsCount(t, post.ID)
		c := e.comment(t, commenter, post.ID, nil, "Bye")
		expectStatus(t, e.api.Delete("/feed/comments/"+c.ID.String(), commenter.bearer), 204)
		if e.commentsCount(t, post.ID) != before {
			t.Error("count must return to its previous value")
		}
		expectError(t, e.api.Delete("/feed/comments/"+c.ID.String(), commenter.bearer), 404, "not_found")
	})

	t.Run("root with replies becomes a tombstone until the last reply goes", func(t *testing.T) {
		before := e.commentsCount(t, post.ID)
		root := e.comment(t, commenter, post.ID, nil, "Question?")
		e.api.Put("/feed/comments/"+root.ID.String()+"/like", author.bearer)
		reply := e.comment(t, author, post.ID, &root.ID, "Answer")
		nested := e.comment(t, commenter, post.ID, &reply.ID, "Thanks")

		expectStatus(t, e.api.Delete("/feed/comments/"+root.ID.String(), commenter.bearer), 204)
		if got := e.commentsCount(t, post.ID); got != before+2 {
			t.Errorf("comments_count = %d, want %d", got, before+2)
		}
		e.assertCounters(t, post.ID)

		page := decode[commentPage](t, e.api.Get("/feed/posts/"+post.ID.String()+"/comments?limit=50", author.bearer), 200)
		var tomb *Comment
		for i := range page.Items {
			if page.Items[i].ID == root.ID {
				tomb = &page.Items[i]
			}
		}
		if tomb == nil || !tomb.Deleted || tomb.Content != nil || tomb.Author != nil || tomb.LikesCount != 0 || tomb.RepliesCount != 2 {
			t.Fatalf("tombstone = %+v", tomb)
		}

		expectError(t, e.api.Delete("/feed/comments/"+root.ID.String(), commenter.bearer), 404, "not_found")
		expectError(t, e.api.Put("/feed/comments/"+root.ID.String()+"/like", author.bearer), 404, "not_found")
		expectError(t, e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, map[string]any{"content": "x", "replyToCommentId": root.ID}), 404, "not_found")

		late := e.comment(t, author, post.ID, &reply.ID, "Still answering inside the thread")
		if *late.ParentID != root.ID {
			t.Errorf("reply inside a tombstoned thread = %+v", late)
		}
		expectStatus(t, e.api.Delete("/feed/comments/"+late.ID.String(), author.bearer), 204)

		expectStatus(t, e.api.Delete("/feed/comments/"+reply.ID.String(), author.bearer), 204)
		var replyTo *uuid.UUID
		_ = e.pool.QueryRow(context.Background(), "SELECT reply_to_comment_id FROM feed_comments WHERE id = $1", nested.ID).Scan(&replyTo)
		if replyTo != nil {
			t.Errorf("reply_to_comment_id = %v, want null after its target was deleted", replyTo)
		}

		expectStatus(t, e.api.Delete("/feed/comments/"+nested.ID.String(), commenter.bearer), 204)
		var exists bool
		_ = e.pool.QueryRow(context.Background(), "SELECT EXISTS (SELECT 1 FROM feed_comments WHERE id = $1)", root.ID).Scan(&exists)
		if exists {
			t.Error("a tombstone without replies must be removed")
		}
		if got := e.commentsCount(t, post.ID); got != before {
			t.Errorf("comments_count = %d, want %d", got, before)
		}
		e.assertCounters(t, post.ID)
	})
}

func TestCommentLikes(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	post := e.createPost(t, author, nil)
	c := e.comment(t, author, post.ID, nil, "Like me")
	path := "/feed/comments/" + c.ID.String() + "/like"

	if got := decode[LikeState](t, e.api.Put(path, author.bearer), 200); got != (LikeState{Liked: true, LikesCount: 1}) {
		t.Errorf("like = %+v", got)
	}
	if got := decode[LikeState](t, e.api.Put(path, author.bearer), 200); got.LikesCount != 1 {
		t.Errorf("second like = %+v", got)
	}
	page := decode[commentPage](t, e.api.Get("/feed/posts/"+post.ID.String()+"/comments", author.bearer), 200)
	if !page.Items[0].LikedByMe {
		t.Error("likedByMe must be true")
	}
	if got := decode[LikeState](t, e.api.Delete(path, author.bearer), 200); got != (LikeState{Liked: false, LikesCount: 0}) {
		t.Errorf("unlike = %+v", got)
	}
	expectError(t, e.api.Put("/feed/comments/"+uuid.NewString()+"/like", author.bearer), 404, "not_found")
	e.assertCounters(t, post.ID)
}

func TestConcurrentComments(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)

	t.Run("simultaneous replies to one root", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		root := e.comment(t, author, post.ID, nil, "Root")
		users := make([]testUser, 20)
		for i := range users {
			users[i] = e.user(t)
		}

		release := e.holdLock(t, "SELECT 1 FROM feed_posts WHERE id = $1 FOR UPDATE", post.ID)
		var wg sync.WaitGroup
		for _, u := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := e.api.Post("/feed/posts/"+post.ID.String()+"/comments", u.bearer, map[string]any{"content": "Reply", "replyToCommentId": root.ID})
				checkStatus(t, rec, 201)
			}()
		}
		waitForLockWait(t, 20, "UPDATE feed_posts SET comments_count")
		release()
		wg.Wait()

		var replies int
		_ = e.pool.QueryRow(context.Background(), "SELECT replies_count FROM feed_comments WHERE id = $1", root.ID).Scan(&replies)
		if replies != 20 {
			t.Errorf("replies_count = %d, want 20", replies)
		}
		e.assertCounters(t, post.ID)
	})

	t.Run("concurrent deletes of a root with replies count once", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		root := e.comment(t, author, post.ID, nil, "Root")
		e.comment(t, author, post.ID, &root.ID, "Reply")

		release := e.holdLock(t, "SELECT 1 FROM feed_posts WHERE id = $1 FOR UPDATE", post.ID)
		codes := make(chan int, 5)
		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes <- e.api.Delete("/feed/comments/"+root.ID.String(), author.bearer).Code
			}()
		}
		waitForLockWait(t, 5, "FROM feed_posts WHERE id = $1 FOR NO KEY UPDATE")
		release()
		wg.Wait()
		close(codes)

		deleted := 0
		for code := range codes {
			switch code {
			case 204:
				deleted++
			case 404:
			default:
				t.Errorf("status %d", code)
			}
		}
		if deleted != 1 {
			t.Errorf("%d deletes succeeded, want 1", deleted)
		}
		if got := e.commentsCount(t, post.ID); got != 1 {
			t.Errorf("comments_count = %d, want 1 live reply", got)
		}
		e.assertCounters(t, post.ID)
	})

	t.Run("replies, comment likes and post delete race", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		root := e.comment(t, author, post.ID, nil, "Root")
		other := e.user(t)

		release := e.holdLock(t, "SELECT 1 FROM feed_posts WHERE id = $1 FOR UPDATE", post.ID)
		var wg sync.WaitGroup
		run := func(f func() int, ok ...int) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				code := f()
				for _, want := range ok {
					if code == want {
						return
					}
				}
				t.Errorf("status %d, want one of %v", code, ok)
			}()
		}
		for range 5 {
			run(func() int {
				return e.api.Post("/feed/posts/"+post.ID.String()+"/comments", other.bearer, map[string]any{"content": "Reply", "replyToCommentId": root.ID}).Code
			}, 201, 404)
			run(func() int { return e.api.Put("/feed/comments/"+root.ID.String()+"/like", other.bearer).Code }, 200, 404)
			run(func() int { return e.api.Put("/feed/posts/"+post.ID.String()+"/like", other.bearer).Code }, 200, 404)
		}
		run(func() int { return e.api.Delete("/feed/posts/"+post.ID.String(), author.bearer).Code }, 204)
		waitForLockWait(t, 11, "feed_posts")
		release()
		wg.Wait()

		var left int
		_ = e.pool.QueryRow(context.Background(), `
			SELECT (SELECT count(*) FROM feed_comments WHERE post_id = $1)
			     + (SELECT count(*) FROM feed_post_likes WHERE post_id = $1)
			     + (SELECT count(*) FROM feed_comment_likes WHERE comment_id = $2)`, post.ID, root.ID).Scan(&left)
		if left != 0 {
			t.Errorf("%d rows outlived the post", left)
		}
	})
}

func TestMoreCommentRaces(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)

	t.Run("replies wait on the root row, not only the post", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		root := e.comment(t, author, post.ID, nil, "Root")

		release := e.holdLock(t, "SELECT 1 FROM feed_comments WHERE id = $1 FOR NO KEY UPDATE", root.ID)
		var wg sync.WaitGroup
		for range 5 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				checkStatus(t, e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, map[string]any{"content": "Reply", "replyToCommentId": root.ID}), 201)
			}()
		}
		waitForLockWait(t, 1, "UPDATE feed_comments SET replies_count = replies_count + 1")
		release()
		wg.Wait()
		e.assertCounters(t, post.ID)
	})

	t.Run("same key comments concurrently create one", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		key := "Idempotency-Key: " + uuid.NewString()
		ids := make(chan uuid.UUID, 10)
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, key, map[string]any{"content": "Once"})
				checkStatus(t, rec, 201)
				ids <- decodeID(t, rec.Body.String())
			}()
		}
		wg.Wait()
		close(ids)
		seen := map[uuid.UUID]bool{}
		for id := range ids {
			seen[id] = true
		}
		if len(seen) != 1 || e.commentsCount(t, post.ID) != 1 {
			t.Errorf("created %d comments, count %d; want 1", len(seen), e.commentsCount(t, post.ID))
		}
	})

	t.Run("comment delete racing its likes", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		c := e.comment(t, author, post.ID, nil, "Liked and deleted")
		users := make([]testUser, 5)
		for i := range users {
			users[i] = e.user(t)
		}

		release := e.holdLock(t, "SELECT 1 FROM feed_comments WHERE id = $1 FOR UPDATE", c.ID)
		var wg sync.WaitGroup
		for _, u := range users {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := e.api.Put("/feed/comments/"+c.ID.String()+"/like", u.bearer)
				if rec.Code != 200 && rec.Code != 404 {
					t.Errorf("like status %d", rec.Code)
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			checkStatus(t, e.api.Delete("/feed/comments/"+c.ID.String(), author.bearer), 204)
		}()
		waitForLockWait(t, 6, "FROM feed_comments WHERE id = $1")
		release()
		wg.Wait()
		e.assertCounters(t, post.ID)
	})

	t.Run("reply racing the delete of its target", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		root := e.comment(t, author, post.ID, nil, "Root")
		target := e.comment(t, author, post.ID, &root.ID, "Target")

		release := e.holdLock(t, "SELECT 1 FROM feed_posts WHERE id = $1 FOR UPDATE", post.ID)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			checkStatus(t, e.api.Delete("/feed/comments/"+target.ID.String(), author.bearer), 204)
		}()
		go func() {
			defer wg.Done()
			rec := e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, map[string]any{"content": "Late", "replyToCommentId": target.ID})
			if rec.Code != 201 && rec.Code != 404 {
				t.Errorf("reply status %d: %s", rec.Code, rec.Body.String())
			}
		}()
		waitForLockWait(t, 2, "feed_posts")
		release()
		wg.Wait()
		e.assertCounters(t, post.ID)
	})
}
