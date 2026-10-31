package feed

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping database integration test because DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to create database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	return pool
}

// fakeMedia keeps media rows in the real table, so foreign keys and the
// ready_media view hold, and skips object storage. Its writes join the
// transaction carried by ctx, like the real module's.
type fakeMedia struct {
	pool            *pgxpool.Pool
	mu              sync.Mutex
	deleted         []uuid.UUID
	beforeMarkReady func(id uuid.UUID)
}

func (f *fakeMedia) CheckUpload(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error {
	var found bool
	err := f.pool.QueryRow(ctx,
		"SELECT EXISTS (SELECT 1 FROM media WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending')",
		mediaID, ownerUserID, kind).Scan(&found)
	if err != nil {
		return err
	}
	if !found {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) MarkReady(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error {
	if f.beforeMarkReady != nil {
		f.beforeMarkReady(mediaID)
	}
	result, err := database.New(f.pool).Conn(ctx).Exec(ctx,
		"UPDATE media SET status = 'ready' WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending'",
		mediaID, ownerUserID, kind)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) URLOf(_ context.Context, ref media.Ref) (string, error) {
	return "https://media.test/" + ref.ObjectKey, nil
}

func (f *fakeMedia) MarkDeleting(ctx context.Context, mediaID uuid.UUID) error {
	result, err := database.New(f.pool).Conn(ctx).Exec(ctx, "UPDATE media SET status = 'deleting' WHERE id = $1 AND status = 'ready'", mediaID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return media.ErrNotFound
	}
	return nil
}

func (f *fakeMedia) Delete(ctx context.Context, mediaID uuid.UUID) error {
	f.mu.Lock()
	f.deleted = append(f.deleted, mediaID)
	f.mu.Unlock()
	_, err := f.pool.Exec(ctx, "DELETE FROM media WHERE id = $1 AND status = 'deleting'", mediaID)
	return err
}

func (e *testEnv) insertMedia(t *testing.T, ownerID uuid.UUID, kind media.Kind) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := e.pool.Exec(context.Background(), `
		INSERT INTO media (id, owner_user_id, kind, object_key, content_type, size_bytes)
		VALUES ($1, $2, $3, $4, 'image/jpeg', 10)`, id, ownerID, kind, string(kind)+"/"+id.String()+".jpg")
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	return id
}

func (e *testEnv) mediaStatus(t *testing.T, id uuid.UUID) string {
	t.Helper()

	var s string
	err := e.pool.QueryRow(context.Background(), "SELECT COALESCE((SELECT status FROM media WHERE id = $1), 'gone')", id).Scan(&s)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type testEnv struct {
	pool   *pgxpool.Pool
	api    humatest.TestAPI
	media  *fakeMedia
	module Module
	users  []uuid.UUID
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
	fm := &fakeMedia{pool: pool}
	m, err := New(pool, fm, jobs.NewRunner(pool, 1, 0))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	usos := map[string]config.UsosCredentials{}
	for _, u := range auth.GetUniversities() {
		if u.AuthType == auth.AuthTypeUSOS {
			usos[string(u.ID)] = config.UsosCredentials{BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"}
		}
	}
	authModule, err := auth.New(pool, &config.Config{UsosConfigs: usos}, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	m.Register(api)

	e := &testEnv{pool: pool, api: api, media: fm, module: m}
	t.Cleanup(e.cleanup)
	return e
}

func (e *testEnv) cleanup() {
	ctx := context.Background()
	for _, query := range []string{
		"DELETE FROM feed_posts WHERE author_id = ANY($1)",
		"DELETE FROM feed_comment_likes WHERE user_id = ANY($1)",
		"DELETE FROM feed_comments WHERE author_id = ANY($1) AND parent_id IS NOT NULL",
		"DELETE FROM feed_comments WHERE author_id = ANY($1)",
		"DELETE FROM feed_post_likes WHERE user_id = ANY($1)",
		"DELETE FROM reports WHERE reporter_id = ANY($1) OR target_author_id = ANY($1)",
		"DELETE FROM users WHERE id = ANY($1)",
	} {
		if _, err := e.pool.Exec(ctx, query, e.users); err != nil {
			panic(err)
		}
	}
}

type testUser struct {
	id     uuid.UUID
	bearer string
}

func (e *testEnv) newUser(t *testing.T, university string) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx,
		"INSERT INTO users (university_id, email, first_name, last_name) VALUES ($1, $2, 'Jan', 'Kowalski') RETURNING id",
		university, "feed-"+uuid.NewString()+"@example.test").Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	e.users = append(e.users, id)

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(database.New(e.pool)).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return testUser{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func (e *testEnv) user(t *testing.T) testUser {
	return e.newUser(t, "umcs")
}

func postBody(overrides map[string]any) map[string]any {
	body := map[string]any{
		"type":       "announcement",
		"university": "umcs",
		"title":      "Room near campus",
		"content":    "Single room available starting next month.",
	}
	for k, v := range overrides {
		body[k] = v
	}
	return body
}

func (e *testEnv) createPost(t *testing.T, author testUser, overrides map[string]any) Post {
	t.Helper()
	return decode[Post](t, e.api.Post("/feed/posts", author.bearer, postBody(overrides)), 201)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, rec.Body.String())
	}
	return v
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	body := decode[map[string]string](t, rec, wantStatus)
	if body["error"] != wantCode {
		t.Errorf(`expected {"error":%q}, got %s`, wantCode, rec.Body.String())
	}
}

// checkStatus is expectStatus for goroutines other than the test's own.
func checkStatus(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Errorf("expected %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
}

func expectStatus(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
}

// waitForLockWait blocks until at least waiters queries containing queryPart
// wait on a row lock, so a test knows the concurrent requests really contend.
func waitForLockWait(t *testing.T, waiters int, queryPart string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Its own connection: the waiting requests may hold every pooled one.
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	for ctx.Err() == nil {
		var waiting int
		err := conn.QueryRow(ctx, `
			SELECT count(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock'
			  AND regexp_replace(query, '\s+', ' ', 'g') LIKE '%' || $1 || '%'`, queryPart).Scan(&waiting)
		if err != nil {
			break
		}
		if waiting >= waiters {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fewer than %d queries containing %q waited for a lock", waiters, queryPart)
}

// assertCounters checks every stored counter of the post and its comments
// against the rows it counts.
func (e *testEnv) assertCounters(t *testing.T, postID uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	var postLikes, likeRows, comments, liveComments int
	err := e.pool.QueryRow(ctx, `
		SELECT p.likes_count,
			(SELECT count(*) FROM feed_post_likes WHERE post_id = p.id),
			p.comments_count,
			(SELECT count(*) FROM feed_comments WHERE post_id = p.id AND deleted_at IS NULL)
		FROM feed_posts p WHERE p.id = $1`, postID).Scan(&postLikes, &likeRows, &comments, &liveComments)
	if err != nil {
		t.Fatalf("read post counters: %v", err)
	}
	if postLikes != likeRows {
		t.Errorf("post likes_count = %d, like rows = %d", postLikes, likeRows)
	}
	if comments != liveComments {
		t.Errorf("post comments_count = %d, live comments = %d", comments, liveComments)
	}

	rows, err := e.pool.Query(ctx, `
		SELECT c.id, c.likes_count,
			(SELECT count(*) FROM feed_comment_likes WHERE comment_id = c.id),
			c.replies_count,
			(SELECT count(*) FROM feed_comments r WHERE r.parent_id = c.id),
			c.deleted_at IS NOT NULL
		FROM feed_comments c WHERE c.post_id = $1`, postID)
	if err != nil {
		t.Fatalf("read comment counters: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var likes, likeRows, replies, replyRows int
		var deleted bool
		if err := rows.Scan(&id, &likes, &likeRows, &replies, &replyRows, &deleted); err != nil {
			t.Fatal(err)
		}
		if deleted && replyRows == 0 {
			t.Errorf("comment %s is a tombstone without replies", id)
		}
		if likes != likeRows {
			t.Errorf("comment %s likes_count = %d, like rows = %d", id, likes, likeRows)
		}
		if replies != replyRows {
			t.Errorf("comment %s replies_count = %d, reply rows = %d", id, replies, replyRows)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
}
