package feed

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) countRows(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

func TestBlockedText(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a post with blocked text in any field is rejected and not stored", func(t *testing.T) {
		author := e.user(t)
		for _, overrides := range []map[string]any{
			{"title": "Kurwa, room near campus"},
			{"content": "Single room, k u r w a, available next month."},
			{"content": "Single room available, sh1t landlord though."},
			{"tags": []string{"housing", "kurw@"}},
		} {
			rec := e.api.Post("/feed/posts", author.bearer, postBody(overrides))
			expectError(t, rec, http.StatusBadRequest, "content_not_allowed")
		}
		if n := e.countRows(t, "SELECT count(*) FROM feed_posts WHERE author_id = $1", author.id); n != 0 {
			t.Errorf("stored %d posts, want 0", n)
		}
	})

	t.Run("a retry with the same idempotency key after a rejection is rejected again", func(t *testing.T) {
		author := e.user(t)
		key := "Idempotency-Key: " + uuid.NewString()
		body := postBody(map[string]any{"title": "Kurwa, room near campus"})

		expectError(t, e.api.Post("/feed/posts", author.bearer, key, body), http.StatusBadRequest, "content_not_allowed")
		expectError(t, e.api.Post("/feed/posts", author.bearer, key, body), http.StatusBadRequest, "content_not_allowed")

		if n := e.countRows(t, "SELECT count(*) FROM feed_posts WHERE author_id = $1", author.id); n != 0 {
			t.Errorf("stored %d posts, want 0", n)
		}
	})

	t.Run("editing a post into blocked text is rejected and the post stays unchanged", func(t *testing.T) {
		author := e.user(t)
		post := e.createPost(t, author, nil)
		path := "/feed/posts/" + post.ID.String()

		for _, edit := range []map[string]any{
			{"title": "Kurwa"},
			{"content": "Single room, but the landlord is a puta."},
			{"tags": []string{"sh1t"}},
		} {
			expectError(t, e.api.Patch(path, author.bearer, edit), http.StatusBadRequest, "content_not_allowed")
		}

		got := decode[Post](t, e.api.Get(path, author.bearer), http.StatusOK)
		if got.Title != post.Title || got.Content != post.Content || len(got.Tags) != len(post.Tags) {
			t.Errorf("post changed: got %+v, want %+v", got, post)
		}
	})

	t.Run("a blocked comment is rejected and an edit into blocked text keeps the comment", func(t *testing.T) {
		author := e.user(t)
		post := e.createPost(t, author, nil)

		rec := e.api.Post("/feed/posts/"+post.ID.String()+"/comments", author.bearer, map[string]any{"content": "f u c k this"})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")
		if n := e.countRows(t, "SELECT count(*) FROM feed_comments WHERE post_id = $1", post.ID); n != 0 {
			t.Errorf("stored %d comments, want 0", n)
		}

		c := e.comment(t, author, post.ID, nil, "Nice room")
		rec = e.api.Patch("/feed/comments/"+c.ID.String(), author.bearer, map[string]any{"content": "хуйня"})
		expectError(t, rec, http.StatusBadRequest, "content_not_allowed")

		var content string
		if err := e.pool.QueryRow(context.Background(), "SELECT content FROM feed_comments WHERE id = $1", c.ID).Scan(&content); err != nil {
			t.Fatalf("read comment: %v", err)
		}
		if content != "Nice room" {
			t.Errorf("comment content = %q, want %q", content, "Nice room")
		}
	})

	t.Run("ordinary student posts in several languages are accepted", func(t *testing.T) {
		author := e.user(t)
		for _, overrides := range []map[string]any{
			{"title": "Pokój blisko kampusu", "content": "Wolny pokój od przyszłego miesiąca, cena do negocjacji."},
			{"title": "Кімната біля кампусу", "content": "Вільна кімната з наступного місяця, ціна договірна."},
			{"title": "Комната у кампуса", "content": "Свободная комната со следующего месяца, цена договорная."},
			{"title": "Habitación cerca del campus", "content": "Mi computadora sirve para estudiar, la disputa terminó."},
			{"title": "Zimmer am Campus", "content": "Freies Zimmer ab nächstem Monat, Schlüssel bei mir."},
		} {
			e.createPost(t, author, overrides)
		}
	})
}
