package feed

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

func TestSearchPosts(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	ctx := context.Background()
	marker := uuid.NewString()
	var ids []uuid.UUID
	for _, fields := range []map[string]any{
		{"title": "ŻÓŁĆ " + marker},
		{"content": "A description with żółć " + marker},
		{"title": "Unrelated " + marker},
	} {
		ids = append(ids, e.createPost(t, author, fields).ID)
	}
	// Shared timestamps exercise the ID tie breaker across matching/nonmatching rows.
	if _, err := e.pool.Exec(ctx, "UPDATE feed_posts SET created_at = '2026-01-01' WHERE author_id = $1", author.id); err != nil {
		t.Fatal(err)
	}
	query := url.Values{"q": {"ZOLC " + marker}, "university": {"umcs"}, "type": {"announcement"}, "from": {"2026-01-01T00:00:00Z"}, "to": {"2026-01-02T00:00:00Z"}, "limit": {"1"}}
	var got []uuid.UUID
	for range 3 {
		page := decode[postPage](t, e.api.Get("/feed/posts?"+query.Encode(), author.bearer), 200)
		for _, p := range page.Items {
			got = append(got, p.ID)
		}
		if page.NextCursor == nil {
			break
		}
		query.Set("cursor", *page.NextCursor)
	}
	want := slices.Clone(ids[:2])
	slices.SortFunc(want, func(a, b uuid.UUID) int { return -strings.Compare(a.String(), b.String()) })
	if !slices.Equal(got, want) {
		t.Fatalf("search pagination = %v, want %v", got, want)
	}

	// Search author names and target university names/IDs, including live profile edits.
	if _, err := e.pool.Exec(ctx, "UPDATE users SET first_name = 'Łukasz', last_name = $2 WHERE id = $1", author.id, marker); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"LUKASZ " + marker, "Curie-Sklodowskiej", "UMCS", "  "} {
		page := decode[postPage](t, e.api.Get("/feed/posts?"+url.Values{"q": {q}, "from": {"2026-01-01T00:00:00Z"}, "to": {"2026-01-02T00:00:00Z"}}.Encode(), author.bearer), 200)
		for _, id := range ids {
			found := slices.ContainsFunc(page.Items, func(p Post) bool { return p.ID == id })
			if !found {
				t.Errorf("q=%q missed %s", q, id)
			}
		}
	}
	for _, q := range []string{"%%%", "___", `\\\`, "％％％", "＿＿＿", "＼＼＼", "!!!", marker + "missing"} {
		page := decode[postPage](t, e.api.Get("/feed/posts?"+url.Values{"q": {q}, "from": {"2026-01-01T00:00:00Z"}, "to": {"2026-01-02T00:00:00Z"}}.Encode(), author.bearer), 200)
		if len(page.Items) != 0 {
			t.Errorf("literal q=%q returned %d posts", q, len(page.Items))
		}
	}
	for _, q := range []string{"a", " ab ", "żó"} {
		expectError(t, e.api.Get("/feed/posts?"+url.Values{"q": {q}}.Encode(), author.bearer), 400, "invalid_input")
	}
	everyone := e.createPost(t, author, map[string]any{"university": "all", "title": "Open day " + marker})
	if _, err := e.pool.Exec(ctx, "UPDATE feed_posts SET created_at = '2025-06-01' WHERE id = $1", everyone.ID); err != nil {
		t.Fatal(err)
	}
	page := decode[postPage](t, e.api.Get("/feed/posts?"+url.Values{"q": {"all"}, "from": {"2025-06-01T00:00:00Z"}, "to": {"2025-06-02T00:00:00Z"}}.Encode(), author.bearer), 200)
	if len(page.Items) != 0 {
		t.Errorf("q=all matched %d posts targeted to everyone", len(page.Items))
	}
	expectError(t, e.api.Get("/feed/posts?q=room"), 401, "unauthorized")
}

func TestPostNavigation(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	marker := uuid.NewString()
	var ids []uuid.UUID
	for range 4 {
		ids = append(ids, e.createPost(t, author, map[string]any{"title": marker}).ID)
	}
	e.createPost(t, author, map[string]any{"title": "Excluded question", "type": "question"})
	if _, err := e.pool.Exec(context.Background(), "UPDATE feed_posts SET created_at = '2026-01-01' WHERE author_id = $1", author.id); err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return -strings.Compare(a.String(), b.String()) })
	query := "?" + url.Values{"q": {marker}, "university": {"umcs"}, "type": {"announcement"}, "from": {"2026-01-01T00:00:00Z"}, "to": {"2026-01-02T00:00:00Z"}}.Encode()
	for i, id := range ids {
		nav := decode[PostNavigation](t, e.api.Get("/feed/posts/"+id.String()+"/navigation"+query, author.bearer), 200)
		if i == 0 {
			if nav.PreviousID != nil {
				t.Error("first has previous")
			}
		} else if nav.PreviousID == nil || *nav.PreviousID != ids[i-1] {
			t.Errorf("previous at %d = %v", i, nav.PreviousID)
		}
		if i == len(ids)-1 {
			if nav.NextID != nil {
				t.Error("last has next")
			}
		} else if nav.NextID == nil || *nav.NextID != ids[i+1] {
			t.Errorf("next at %d = %v", i, nav.NextID)
		}
	}
	path := "/feed/posts/" + ids[0].String() + "/navigation"
	expectError(t, e.api.Get(path), 401, "unauthorized")
	expectError(t, e.api.Get(path+"?type=question", author.bearer), 404, "not_found")
	expectError(t, e.api.Get(path+"?university=unknown", author.bearer), 400, "invalid_input")
	expectError(t, e.api.Get(path+"?from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z", author.bearer), 400, "invalid_input")
	expectStatus(t, e.api.Delete("/feed/posts/"+ids[1].String(), author.bearer), 204)
	nav := decode[PostNavigation](t, e.api.Get(path+query, author.bearer), 200)
	if nav.NextID == nil || *nav.NextID != ids[2] {
		t.Fatalf("deleted neighbor returned: %+v", nav)
	}
	expectError(t, e.api.Get("/feed/posts/"+ids[1].String()+"/navigation"+query, author.bearer), 404, "not_found")
	for _, id := range ids[2:] {
		expectStatus(t, e.api.Delete("/feed/posts/"+id.String(), author.bearer), 204)
	}
	nav = decode[PostNavigation](t, e.api.Get(path+query, author.bearer), 200)
	if nav.NextID != nil || nav.PreviousID != nil {
		t.Errorf("single post = %+v", nav)
	}
}

func TestFeaturedPosts(t *testing.T) {
	e := newTestEnv(t)
	author := e.user(t)
	viewer := e.newUser(t, "kul")
	ctx := context.Background()
	var weekStart time.Time
	if err := e.pool.QueryRow(ctx, `SELECT date_trunc('week', NOW() AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'Europe/Warsaw'`).Scan(&weekStart); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := e.pool.Exec(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	var likers []testUser
	for range 6 {
		likers = append(likers, e.user(t))
	}
	like := func(id uuid.UUID, users ...testUser) {
		t.Helper()
		for _, liker := range users {
			decode[LikeState](t, e.api.Put("/feed/posts/"+id.String()+"/like", liker.bearer), 200)
		}
	}
	moveToLastWeek := func(postID uuid.UUID, liker testUser) {
		t.Helper()
		exec("UPDATE feed_post_likes SET created_at = $3 WHERE post_id = $1 AND user_id = $2", postID, liker.id, weekStart.Add(-time.Microsecond))
		exec("UPDATE feed_post_weekly_likes SET week = week - 7 WHERE post_id = $1", postID)
	}
	weekly := func(postID uuid.UUID) map[string]int {
		t.Helper()
		rows, err := e.pool.Query(ctx, `SELECT CASE WHEN week = $2::date THEN 'this' ELSE 'last' END, likes
			FROM feed_post_weekly_likes WHERE post_id = $1`, postID, weekStart.In(time.UTC).Add(2*time.Hour).Format("2006-01-02"))
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		counts := map[string]int{}
		for rows.Next() {
			var week string
			var likes int
			if err := rows.Scan(&week, &likes); err != nil {
				t.Fatal(err)
			}
			counts[week] = likes
		}
		return counts
	}

	var want []uuid.UUID
	// Any post age/type qualifies, including an event whose date has passed.
	for i, count := range []int{6, 5, 4, 3, 1, 1} {
		fields := map[string]any{}
		if i == 0 {
			fields["mediaIds"] = []uuid.UUID{e.insertMedia(t, author.id, media.KindFeed)}
		}
		if i == 1 {
			fields["type"] = "event"
			fields["eventDate"] = weekStart.Add(-time.Hour)
		}
		if i == 2 {
			fields["type"] = "question"
		}
		post := e.createPost(t, author, fields)
		want = append(want, post.ID)
		createdAt := weekStart.Add(time.Duration(i-10) * time.Hour)
		if i >= 4 {
			// Same count and timestamp: ID breaks the tie deterministically.
			createdAt = weekStart.Add(-time.Hour)
		}
		exec("UPDATE feed_posts SET created_at = $2 WHERE id = $1", post.ID, createdAt)
		like(post.ID, likers[:count]...)
	}
	slices.SortFunc(want[4:], func(a, b uuid.UUID) int { return -strings.Compare(a.String(), b.String()) })

	lastWeek := e.createPost(t, author, nil)
	like(lastWeek.ID, likers[0])
	moveToLastWeek(lastWeek.ID, likers[0])
	e.createPost(t, author, nil) // No likes: never featured.

	type cards struct {
		Items []FeaturedPost `json:"items"`
	}
	get := func() []FeaturedPost {
		t.Helper()
		return decode[cards](t, e.api.Get("/feed/posts/featured", viewer.bearer), 200).Items
	}
	items := get()
	if len(items) != 5 {
		t.Fatalf("cards = %+v", items)
	}
	for i, card := range items {
		if card.ID != want[i] || card.Title == "" || card.University != "umcs" {
			t.Errorf("card %d = %+v", i, card)
		}
		decode[Post](t, e.api.Get("/feed/posts/"+card.ID.String(), viewer.bearer), 200)
	}
	if items[0].ImageURL == nil || items[2].ImageURL != nil || items[0].EventDate != nil || items[1].EventDate == nil {
		t.Error("incorrect image/event date null handling")
	}

	t.Run("a repeated like or unlike changes nothing", func(t *testing.T) {
		like(want[3], likers[0])
		expectStatus(t, e.api.Delete("/feed/posts/"+want[5].String()+"/like", likers[5].bearer), 200)
		if got := weekly(want[3]); got["this"] != 3 {
			t.Errorf("weekly likes after a repeated like = %v", got)
		}
	})

	t.Run("unlike reranks", func(t *testing.T) {
		expectStatus(t, e.api.Delete("/feed/posts/"+want[0].String()+"/like", likers[0].bearer), 200)
		items := get()
		if items[0].ID != want[1] || items[1].ID != want[0] {
			t.Fatalf("unlike did not rerank: %+v", items)
		}
	})

	t.Run("taking back a like from last week leaves this week alone", func(t *testing.T) {
		post := e.createPost(t, author, nil)
		like(post.ID, likers[0])
		moveToLastWeek(post.ID, likers[0])
		like(post.ID, likers[1], likers[2])
		expectStatus(t, e.api.Delete("/feed/posts/"+post.ID.String()+"/like", likers[0].bearer), 200)
		if got := weekly(post.ID); got["this"] != 2 || got["last"] != 0 {
			t.Errorf("weekly likes = %v, want this week 2 and last week 0", got)
		}
		expectStatus(t, e.api.Delete("/feed/posts/"+post.ID.String(), author.bearer), 204)
	})

	t.Run("deleted posts and past weeks are not featured", func(t *testing.T) {
		expectStatus(t, e.api.Delete("/feed/posts/"+want[1].String(), author.bearer), 204)
		for _, card := range get() {
			if card.ID == want[1] || card.ID == lastWeek.ID {
				t.Errorf("unexpected card %v", card.ID)
			}
		}
	})

	t.Run("the prune job removes past weeks only", func(t *testing.T) {
		if err := e.module.pruneWeeklyLikes(ctx, PruneWeeklyLikesArgs{}); err != nil {
			t.Fatal(err)
		}
		if got := weekly(lastWeek.ID); len(got) != 0 {
			t.Errorf("last week's row survived: %v", got)
		}
		if got := weekly(want[2]); got["this"] != 4 {
			t.Errorf("this week's row = %v", got)
		}
	})

	t.Run("an empty section is an empty list", func(t *testing.T) {
		for _, id := range append([]uuid.UUID{want[0]}, want[2:]...) {
			expectStatus(t, e.api.Delete("/feed/posts/"+id.String(), author.bearer), 204)
		}
		items := get()
		if items == nil || len(items) != 0 {
			t.Errorf("empty cards = %+v", items)
		}
	})

	expectError(t, e.api.Get("/feed/posts/featured"), 401, "unauthorized")
}
