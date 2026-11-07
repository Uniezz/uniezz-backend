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
	for _, q := range []string{"%", "_", `\`, "％", "＿", "＼", "!", marker + "missing"} {
		page := decode[postPage](t, e.api.Get("/feed/posts?"+url.Values{"q": {q}, "from": {"2026-01-01T00:00:00Z"}, "to": {"2026-01-02T00:00:00Z"}}.Encode(), author.bearer), 200)
		if len(page.Items) != 0 {
			t.Errorf("literal q=%q returned %d posts", q, len(page.Items))
		}
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
	var weekStart, weekEnd time.Time
	if err := e.pool.QueryRow(ctx, `SELECT date_trunc('week', NOW() AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'Europe/Warsaw', (date_trunc('week', NOW() AT TIME ZONE 'Europe/Warsaw') + interval '1 week') AT TIME ZONE 'Europe/Warsaw'`).Scan(&weekStart, &weekEnd); err != nil {
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
	like := func(id uuid.UUID, count int) {
		t.Helper()
		for _, liker := range likers[:count] {
			decode[LikeState](t, e.api.Put("/feed/posts/"+id.String()+"/like", liker.bearer), 200)
		}
		exec("UPDATE feed_post_likes SET created_at = $2 WHERE post_id = $1", id, weekStart)
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
		exec("UPDATE feed_posts SET created_at = $2 WHERE id = $1", post.ID, weekStart.Add(time.Duration(i-10)*time.Hour))
		like(post.ID, count)
	}
	// Same count and timestamp: ID breaks the tie deterministically.
	exec("UPDATE feed_posts SET created_at = $2 WHERE id = ANY($1)", want[4:], weekStart.Add(-time.Hour))
	slices.SortFunc(want[4:], func(a, b uuid.UUID) int { return -strings.Compare(a.String(), b.String()) })
	for _, date := range []time.Time{weekStart.Add(-time.Microsecond), weekEnd} {
		post := e.createPost(t, author, nil)
		like(post.ID, 6)
		exec("UPDATE feed_post_likes SET created_at = $2 WHERE post_id = $1", post.ID, date)
	}
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
	expectStatus(t, e.api.Delete("/feed/posts/"+want[0].String()+"/like", likers[0].bearer), 200)
	items = get()
	if items[0].ID != want[1] || items[1].ID != want[0] {
		t.Fatalf("unlike did not rerank: %+v", items)
	}
	expectStatus(t, e.api.Delete("/feed/posts/"+want[0].String(), author.bearer), 204)
	exec("UPDATE feed_post_likes SET created_at = $2 WHERE post_id = $1", want[1], weekStart.Add(-time.Microsecond))
	items = get()
	if len(items) != 4 || items[0].ID != want[2] {
		t.Fatalf("previous-week/deleted posts returned: %+v", items)
	}
	for _, id := range want[2:] {
		expectStatus(t, e.api.Delete("/feed/posts/"+id.String(), author.bearer), 204)
	}
	items = get()
	if items == nil || len(items) != 0 {
		t.Errorf("empty cards = %+v", items)
	}
	expectError(t, e.api.Get("/feed/posts/featured"), 401, "unauthorized")
}
