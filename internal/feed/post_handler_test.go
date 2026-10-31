package feed

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

func TestCreatePost(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)

	t.Run("creates a post with media in order", func(t *testing.T) {
		first := e.insertMedia(t, author.id, media.KindFeed)
		second := e.insertMedia(t, author.id, media.KindFeed)

		post := e.createPost(t, author, map[string]any{
			"title":    " <b>Room</b> near campus ",
			"tags":     []string{"Housing", "housing", "Lublin"},
			"mediaIds": []uuid.UUID{second, first},
		})

		if post.Title != "Room near campus" {
			t.Errorf("title = %q, want HTML stripped and trimmed", post.Title)
		}
		if strings.Join(post.Tags, ",") != "Housing,Lublin" {
			t.Errorf("tags = %v, want deduplicated", post.Tags)
		}
		want := []string{"https://media.test/feed/" + second.String() + ".jpg", "https://media.test/feed/" + first.String() + ".jpg"}
		if strings.Join(post.MediaURLs, " ") != strings.Join(want, " ") {
			t.Errorf("mediaUrls = %v, want %v", post.MediaURLs, want)
		}
		if post.Author.ID != author.id || post.Author.UniversityID != "umcs" || *post.Author.FirstName != "Jan" {
			t.Errorf("author = %+v", post.Author)
		}
		if post.LikesCount != 0 || post.CommentsCount != 0 || post.LikedByMe || post.EditedAt != nil {
			t.Errorf("new post counters = %+v", post)
		}
		if e.mediaStatus(t, first) != "ready" || e.mediaStatus(t, second) != "ready" {
			t.Error("attached media must be ready")
		}
	})

	t.Run("targets everyone", func(t *testing.T) {
		post := e.createPost(t, author, map[string]any{"university": "all"})
		if post.University != "all" {
			t.Errorf("university = %q", post.University)
		}
	})

	t.Run("event with a date", func(t *testing.T) {
		date := time.Date(2026, 12, 1, 18, 0, 0, 0, time.UTC)
		post := e.createPost(t, author, map[string]any{"type": "event", "eventDate": date})
		if post.EventDate == nil || !post.EventDate.Equal(date) {
			t.Errorf("eventDate = %v, want %v", post.EventDate, date)
		}
	})

	t.Run("rejects invalid input", func(t *testing.T) {
		cases := map[string]map[string]any{
			"unknown type":             {"type": "sale"},
			"short title":              {"title": "<i>ab</i>"},
			"short content":            {"content": "too short"},
			"date on non-event":        {"eventDate": time.Now()},
			"duplicate media":          {"mediaIds": []uuid.UUID{uuid.Nil, uuid.Nil}},
			"too many tags":            {"tags": []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}},
			"tag of only HTML":         {"tags": []string{"<b></b>"}},
			"content over 5000 runes":  {"content": strings.Repeat("ł", 5001)},
			"title over 150 runes":     {"title": strings.Repeat("ł", 151)},
			"more than ten media":      {"mediaIds": manyIDs(11)},
			"missing required content": {"content": nil},
		}
		for name, override := range cases {
			t.Run(name, func(t *testing.T) {
				body := postBody(override)
				for k, v := range override {
					if v == nil {
						delete(body, k)
					}
				}
				expectError(t, e.api.Post("/feed/posts", author.bearer, body), 400, "invalid_input")
			})
		}
	})

	t.Run("rejects another university", func(t *testing.T) {
		expectError(t, e.api.Post("/feed/posts", author.bearer, postBody(map[string]any{"university": "kul"})), 400, "invalid_university")
	})

	t.Run("rejects media of another user, another kind or already used", func(t *testing.T) {
		other := e.user(t)
		cases := map[string]uuid.UUID{
			"other owner": e.insertMedia(t, other.id, media.KindFeed),
			"avatar kind": e.insertMedia(t, author.id, media.KindAvatar),
			"unknown":     uuid.New(),
		}
		for name, id := range cases {
			t.Run(name, func(t *testing.T) {
				expectError(t, e.api.Post("/feed/posts", author.bearer, postBody(map[string]any{"mediaIds": []uuid.UUID{id}})), 400, "invalid_media")
			})
		}
	})

	t.Run("a failed create leaves no post and media pending", func(t *testing.T) {
		good := e.insertMedia(t, author.id, media.KindFeed)
		taken := e.insertMedia(t, author.id, media.KindFeed)

		var postsBefore int
		_ = e.pool.QueryRow(context.Background(), "SELECT count(*) FROM feed_posts WHERE author_id = $1", author.id).Scan(&postsBefore)

		// taken passes the pre-check, then another request claims it before
		// this one's transaction marks it ready.
		e.media.beforeMarkReady = func(id uuid.UUID) {
			if id == taken {
				_, _ = e.pool.Exec(context.Background(), "UPDATE media SET status = 'ready' WHERE id = $1", taken)
			}
		}
		t.Cleanup(func() { e.media.beforeMarkReady = nil })

		rec := e.api.Post("/feed/posts", author.bearer, postBody(map[string]any{"mediaIds": []uuid.UUID{good, taken}}))
		expectError(t, rec, 400, "invalid_media")

		var postsAfter int
		_ = e.pool.QueryRow(context.Background(), "SELECT count(*) FROM feed_posts WHERE author_id = $1", author.id).Scan(&postsAfter)
		if postsAfter != postsBefore {
			t.Errorf("posts = %d, want %d", postsAfter, postsBefore)
		}
		if got := e.mediaStatus(t, good); got != "pending" {
			t.Errorf("first media status = %q, want pending after rollback", got)
		}
	})

	t.Run("requires a session", func(t *testing.T) {
		expectError(t, e.api.Post("/feed/posts", postBody(nil)), 401, "unauthorized")
	})
}

func manyIDs(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.New()
	}
	return ids
}

func TestCreatePostIdempotency(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)

	t.Run("same key and body returns the first post", func(t *testing.T) {
		key := "Idempotency-Key: " + uuid.NewString()
		first := decode[Post](t, e.api.Post("/feed/posts", author.bearer, key, postBody(nil)), 201)

		rec := e.api.Post("/feed/posts", author.bearer, key, postBody(nil))
		second := decode[Post](t, rec, 201)
		if second.ID != first.ID {
			t.Errorf("replay created %s, want %s", second.ID, first.ID)
		}
		if rec.Header().Get("Idempotent-Replayed") != "true" {
			t.Errorf("Idempotent-Replayed = %q", rec.Header().Get("Idempotent-Replayed"))
		}
	})

	t.Run("same key with another body is rejected", func(t *testing.T) {
		key := "Idempotency-Key: " + uuid.NewString()
		e.createPostWithKey(t, author, key, nil)
		expectError(t, e.api.Post("/feed/posts", author.bearer, key, postBody(map[string]any{"title": "Another title"})), 422, "idempotency_key_reused")
	})

	t.Run("keys are per user", func(t *testing.T) {
		key := "Idempotency-Key: " + uuid.NewString()
		first := e.createPostWithKey(t, author, key, nil)
		other := e.createPostWithKey(t, e.user(t), key, nil)
		if first.ID == other.ID {
			t.Error("another user's request with the same key must create its own post")
		}
	})

	t.Run("replay after an edit returns the current post", func(t *testing.T) {
		key := "Idempotency-Key: " + uuid.NewString()
		first := e.createPostWithKey(t, author, key, nil)
		decode[Post](t, e.api.Patch("/feed/posts/"+first.ID.String(), author.bearer, map[string]any{"title": "Edited title"}), 200)

		replay := decode[Post](t, e.api.Post("/feed/posts", author.bearer, key, postBody(nil)), 201)
		if replay.ID != first.ID || replay.Title != "Edited title" || replay.EditedAt == nil {
			t.Errorf("replay = %+v, want the edited post", replay)
		}
	})

	t.Run("replay after delete is gone", func(t *testing.T) {
		key := "Idempotency-Key: " + uuid.NewString()
		first := e.createPostWithKey(t, author, key, nil)
		expectStatus(t, e.api.Delete("/feed/posts/"+first.ID.String(), author.bearer), 204)
		expectError(t, e.api.Post("/feed/posts", author.bearer, key, postBody(nil)), 410, "resource_deleted")
	})

	t.Run("concurrent requests with one key create one post", func(t *testing.T) {
		key := "Idempotency-Key: " + uuid.NewString()
		ids := make(chan uuid.UUID, 10)
		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := e.api.Post("/feed/posts", author.bearer, key, postBody(nil))
				if rec.Code != 201 {
					t.Errorf("status %d: %s", rec.Code, rec.Body.String())
					return
				}
				ids <- decodeID(t, rec.Body.String())
			}()
		}
		wg.Wait()
		close(ids)

		seen := map[uuid.UUID]bool{}
		for id := range ids {
			seen[id] = true
		}
		if len(seen) != 1 {
			t.Errorf("created %d posts, want 1", len(seen))
		}
	})

	t.Run("a request waiting on the key compares fingerprints after the winner commits", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			sameBody   bool
			wantStatus int
		}{
			{"same body replays", true, 201},
			{"other body is rejected", false, 422},
		} {
			t.Run(tc.name, func(t *testing.T) {
				key := uuid.NewString()
				winnerInput, err := postInput{Type: "announcement", University: "umcs", Title: "Winner title", Content: "Single room available starting next month."}.clean()
				if err != nil {
					t.Fatal(err)
				}
				fp, err := fingerprint(winnerInput)
				if err != nil {
					t.Fatal(err)
				}
				winnerID := uuid.New()

				ctx := context.Background()
				tx, err := e.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = tx.Rollback(ctx) }()
				_, err = tx.Exec(ctx, `INSERT INTO feed_request_keys (user_id, operation, key, fingerprint, resource_id) VALUES ($1, $2, $3, $4, $5)`,
					author.id, opCreatePost, key, fp, winnerID)
				if err != nil {
					t.Fatal(err)
				}

				body := postBody(map[string]any{"title": "Winner title"})
				if !tc.sameBody {
					body = postBody(map[string]any{"title": "Loser title"})
				}
				result := make(chan *httpResult, 1)
				go func() {
					rec := e.api.Post("/feed/posts", author.bearer, "Idempotency-Key: "+key, body)
					result <- &httpResult{code: rec.Code, body: rec.Body.String()}
				}()
				waitForLockWait(t, 1, "INSERT INTO feed_request_keys")

				_, err = tx.Exec(ctx, `INSERT INTO feed_posts (id, author_id, university_id, type, title, content) VALUES ($1, $2, 'umcs', 'announcement', 'Winner title', 'Single room available starting next month.')`,
					winnerID, author.id)
				if err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}

				got := <-result
				if got.code != tc.wantStatus {
					t.Fatalf("status = %d, want %d: %s", got.code, tc.wantStatus, got.body)
				}
				if tc.sameBody && decodeID(t, got.body) != winnerID {
					t.Errorf("replayed %s, want the winner %s", got.body, winnerID)
				}
				var posts int
				_ = e.pool.QueryRow(ctx, "SELECT count(*) FROM feed_posts WHERE author_id = $1 AND title IN ('Winner title', 'Loser title')", author.id).Scan(&posts)
				if posts != 1 {
					t.Errorf("posts = %d, want only the winner", posts)
				}
				_, _ = e.pool.Exec(ctx, "DELETE FROM feed_posts WHERE id = $1", winnerID)
			})
		}
	})
}

type httpResult struct {
	code int
	body string
}

func decodeID(t *testing.T, body string) uuid.UUID {
	t.Helper()
	i := strings.Index(body, `"id":"`)
	if i < 0 {
		t.Fatalf("no id in %s", body)
	}
	id, err := uuid.Parse(body[i+6 : i+42])
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *testEnv) createPostWithKey(t *testing.T, author testUser, key string, overrides map[string]any) Post {
	t.Helper()
	return decode[Post](t, e.api.Post("/feed/posts", author.bearer, key, postBody(overrides)), 201)
}

type postPage struct {
	Items      []Post  `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func TestListPosts(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	tag := uuid.NewString()[:8]

	created := map[string]uuid.UUID{}
	for _, p := range []struct{ name, university, typ string }{
		{"umcs-announcement", "umcs", "announcement"},
		{"umcs-question", "umcs", "question"},
		{"all-event", "all", "event"},
		{"kul-question", "kul", "question"},
	} {
		var id uuid.UUID
		err := e.pool.QueryRow(context.Background(), `
			INSERT INTO feed_posts (author_id, university_id, type, title, content, tags)
			VALUES ($1, $2, $3, $4, 'Some content for the post.', ARRAY[$5]) RETURNING id`,
			author.id, p.university, p.typ, p.name, tag).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		created[p.name] = id
	}
	// All four share one timestamp, so only the id orders them.
	_, err := e.pool.Exec(context.Background(), "UPDATE feed_posts SET created_at = '2026-01-01T00:00:00Z' WHERE author_id = $1", author.id)
	if err != nil {
		t.Fatal(err)
	}

	listAll := func(t *testing.T, query url.Values) []string {
		t.Helper()
		query.Set("from", "2026-01-01T00:00:00Z")
		query.Set("to", "2026-01-02T00:00:00Z")
		query.Set("limit", "1")
		var names []string
		for range 10 {
			page := decode[postPage](t, e.api.Get("/feed/posts?"+query.Encode(), author.bearer), 200)
			for _, p := range page.Items {
				if p.Author.ID == author.id {
					names = append(names, p.Title)
				}
			}
			if page.NextCursor == nil {
				return names
			}
			query.Set("cursor", *page.NextCursor)
		}
		t.Fatal("pagination did not end")
		return nil
	}

	t.Run("pages through equal timestamps by id", func(t *testing.T) {
		names := listAll(t, url.Values{})
		if len(names) != 4 {
			t.Fatalf("got %v, want all four posts once", names)
		}
		for i := 1; i < len(names); i++ {
			if created[names[i-1]].String() < created[names[i]].String() {
				t.Errorf("order %v is not by id descending", names)
			}
		}
	})

	t.Run("filters", func(t *testing.T) {
		cases := []struct {
			query url.Values
			want  []string
		}{
			{url.Values{"university": {"umcs"}}, []string{"all-event", "umcs-announcement", "umcs-question"}},
			{url.Values{"university": {"all"}}, []string{"all-event"}},
			{url.Values{"type": {"question"}}, []string{"kul-question", "umcs-question"}},
			{url.Values{"university": {"kul"}, "type": {"question"}}, []string{"kul-question"}},
		}
		for _, tc := range cases {
			t.Run(tc.query.Encode(), func(t *testing.T) {
				got := listAll(t, tc.query)
				if strings.Join(sorted(got), ",") != strings.Join(tc.want, ",") {
					t.Errorf("got %v, want %v", got, tc.want)
				}
			})
		}
	})

	t.Run("date range excludes posts outside it", func(t *testing.T) {
		page := decode[postPage](t, e.api.Get("/feed/posts?from=2026-01-01T00:00:01Z&to=2026-01-02T00:00:00Z&limit=50", author.bearer), 200)
		for _, p := range page.Items {
			if p.Author.ID == author.id {
				t.Errorf("post %s is before from", p.Title)
			}
		}
	})

	t.Run("rejects bad queries", func(t *testing.T) {
		expectError(t, e.api.Get("/feed/posts?cursor=not-a-cursor", author.bearer), 400, "invalid_cursor")
		expectError(t, e.api.Get("/feed/posts?university=nowhere", author.bearer), 400, "invalid_input")
		expectError(t, e.api.Get("/feed/posts?from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", author.bearer), 400, "invalid_input")
		expectError(t, e.api.Get("/feed/posts?limit=51", author.bearer), 400, "invalid_input")
	})

	t.Run("shows likedByMe for the viewer", func(t *testing.T) {
		viewer := e.user(t)
		_, err := e.pool.Exec(context.Background(), "INSERT INTO feed_post_likes (post_id, user_id) VALUES ($1, $2)", created["kul-question"], viewer.id)
		if err != nil {
			t.Fatal(err)
		}
		post := decode[Post](t, e.api.Get("/feed/posts/"+created["kul-question"].String(), viewer.bearer), 200)
		if !post.LikedByMe {
			t.Error("viewer liked the post")
		}
		post = decode[Post](t, e.api.Get("/feed/posts/"+created["kul-question"].String(), author.bearer), 200)
		if post.LikedByMe {
			t.Error("author did not like the post")
		}
	})
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestGetPost(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.user(t)
	expectError(t, e.api.Get("/feed/posts/"+uuid.NewString(), viewer.bearer), 404, "not_found")
	expectError(t, e.api.Get("/feed/posts/not-a-uuid", viewer.bearer), 400, "invalid_input")
}

func TestUpdatePost(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	post := e.createPost(t, author, nil)
	path := "/feed/posts/" + post.ID.String()

	t.Run("author edits sent fields only", func(t *testing.T) {
		got := decode[Post](t, e.api.Patch(path, author.bearer, map[string]any{"content": "New content of the post."}), 200)
		if got.Content != "New content of the post." || got.Title != post.Title || got.EditedAt == nil {
			t.Errorf("edited = %+v", got)
		}
	})

	t.Run("event date only on events", func(t *testing.T) {
		expectError(t, e.api.Patch(path, author.bearer, map[string]any{"eventDate": time.Now()}), 400, "invalid_input")
	})

	t.Run("others are forbidden", func(t *testing.T) {
		expectError(t, e.api.Patch(path, e.user(t).bearer, map[string]any{"title": "Hijacked"}), 403, "forbidden")
	})

	t.Run("missing post", func(t *testing.T) {
		expectError(t, e.api.Patch("/feed/posts/"+uuid.NewString(), author.bearer, map[string]any{"title": "Nothing here"}), 404, "not_found")
	})
}

func TestDeletePost(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	mediaID := e.insertMedia(t, author.id, media.KindFeed)
	post := e.createPost(t, author, map[string]any{"mediaIds": []uuid.UUID{mediaID}})
	path := "/feed/posts/" + post.ID.String()

	expectError(t, e.api.Delete(path, e.user(t).bearer), 403, "forbidden")
	expectStatus(t, e.api.Delete(path, author.bearer), 204)
	expectError(t, e.api.Delete(path, author.bearer), 404, "not_found")
	expectError(t, e.api.Get(path, author.bearer), 404, "not_found")

	if got := e.mediaStatus(t, mediaID); got != "gone" {
		t.Errorf("media status = %q, want deleted", got)
	}
}

func TestEmptyPatchKeepsPostUnedited(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	post := e.createPost(t, author, nil)

	got := decode[Post](t, e.api.Patch("/feed/posts/"+post.ID.String(), author.bearer, map[string]any{}), 200)
	if got.EditedAt != nil {
		t.Errorf("editedAt = %v, want null after an empty edit", got.EditedAt)
	}
}

func TestSameMediaInOppositeOrder(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	a := e.insertMedia(t, author.id, media.KindFeed)
	b := e.insertMedia(t, author.id, media.KindFeed)

	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for _, ids := range [][]uuid.UUID{{a, b}, {b, a}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- e.api.Post("/feed/posts", author.bearer, postBody(map[string]any{"mediaIds": ids})).Code
		}()
	}
	wg.Wait()
	close(codes)

	created := 0
	for code := range codes {
		switch code {
		case 201:
			created++
		case 400:
		default:
			t.Errorf("status %d, want 201 or 400 invalid_media", code)
		}
	}
	if created != 1 {
		t.Errorf("%d posts got the same media, want 1", created)
	}
}
