package chat

import (
	"context"
	"encoding/json"
	"net/http"
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
	"github.com/Uniezz/uniezz-backend/internal/push"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
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

type fakeMedia struct {
	pool     *pgxpool.Pool
	mu       sync.Mutex
	deleted  []uuid.UUID
	imported []string
}

func (f *fakeMedia) CheckUpload(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error {
	var found bool
	err := f.pool.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM media WHERE id = $1 AND owner_user_id = $2 AND kind = $3 AND status = 'pending')`,
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

func (f *fakeMedia) MarkDeletingMany(ctx context.Context, mediaIDs []uuid.UUID) error {
	_, err := database.New(f.pool).Conn(ctx).Exec(ctx, "UPDATE media SET status = 'deleting' WHERE id = ANY($1) AND status = 'ready'", mediaIDs)
	return err
}

func (f *fakeMedia) Delete(ctx context.Context, mediaID uuid.UUID) error {
	f.mu.Lock()
	f.deleted = append(f.deleted, mediaID)
	f.mu.Unlock()
	_, err := f.pool.Exec(ctx, "DELETE FROM media WHERE id = $1 AND status = 'deleting'", mediaID)
	return err
}

func (f *fakeMedia) URLOf(_ context.Context, ref media.Ref) (string, error) {
	return "https://media.test/" + ref.ObjectKey, nil
}

func (f *fakeMedia) Import(ctx context.Context, ownerUserID uuid.UUID, kind media.Kind, sourceURL string) (uuid.UUID, error) {
	f.mu.Lock()
	f.imported = append(f.imported, sourceURL)
	f.mu.Unlock()
	id := uuid.New()
	_, err := f.pool.Exec(ctx, `
		INSERT INTO media (id, owner_user_id, kind, object_key, content_type, size_bytes, status)
		VALUES ($1, $2, $3, $4, 'image/png', 10, 'ready')`, id, ownerUserID, kind, string(kind)+"/"+id.String()+".png")
	return id, err
}

func (f *fakeMedia) wasDeleted(id uuid.UUID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, d := range f.deleted {
		if d == id {
			return true
		}
	}
	return false
}

type testEnv struct {
	pool   *pgxpool.Pool
	api    humatest.TestAPI
	auth   auth.Module
	media  *fakeMedia
	push   *fakePush
	module Module
	now    time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
	e := &testEnv{pool: pool, media: &fakeMedia{pool: pool}, push: &fakePush{}, now: time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)}
	module, err := New(pool, e.media, e.push, jobs.NewRunner(pool, 1, 0))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	e.module = module
	e.module.now = func() time.Time { return e.now }

	authModule, err := auth.New(pool, &config.Config{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}
	e.auth = authModule

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	e.module.Register(api)
	e.api = api
	return e
}

type testUser struct {
	id        uuid.UUID
	sessionID uuid.UUID
	token     string
	bearer    string
}

func (e *testEnv) user(t *testing.T) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx, `
		INSERT INTO users (university_id, email, first_name, last_name)
		VALUES ('umcs', $1, 'Jan', 'Kowalski') RETURNING id`,
		"chat-"+uuid.NewString()+"@example.test").Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = e.pool.Exec(ctx, "DELETE FROM chat_conversations WHERE id IN (SELECT conversation_id FROM chat_members WHERE user_id = $1)", id)
		_, _ = e.pool.Exec(ctx, "DELETE FROM chat_messages WHERE sender_id = $1", id)
		_, _ = e.pool.Exec(ctx, "DELETE FROM reports WHERE reporter_id = $1 OR target_author_id = $1", id)
		_, _ = e.pool.Exec(ctx, "DELETE FROM rate_limits WHERE key = ANY($1)", []string{"chat-send:" + id.String(), "chat-group:" + id.String(), "chat-request:" + id.String()})
		_, _ = e.pool.Exec(ctx, "DELETE FROM media WHERE owner_user_id = $1", id)
		_, _ = e.pool.Exec(ctx, "DELETE FROM users WHERE id = $1", id)
	})

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	session, err := auth.NewSessionRepository(database.New(e.pool)).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return testUser{id: id, sessionID: session.ID, token: token.String(), bearer: "Authorization: Bearer " + token.String()}
}

func (e *testEnv) connect(t *testing.T, a, b testUser) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(),
		"INSERT INTO connections (user_low, user_high, mode) VALUES (LEAST($1::uuid, $2::uuid), GREATEST($1::uuid, $2::uuid), 'friends') ON CONFLICT DO NOTHING", a.id, b.id)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { e.disconnect(a, b) })
}

func (e *testEnv) disconnect(a, b testUser) {
	_, _ = e.pool.Exec(context.Background(),
		"DELETE FROM connections WHERE user_low = LEAST($1::uuid, $2::uuid) AND user_high = GREATEST($1::uuid, $2::uuid)", a.id, b.id)
}

func (e *testEnv) block(t *testing.T, blocker, blocked testUser) {
	t.Helper()
	if _, err := e.pool.Exec(context.Background(), "INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", blocker.id, blocked.id); err != nil {
		t.Fatalf("block: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2", blocker.id, blocked.id)
	})
}

func (e *testEnv) friends(t *testing.T) (testUser, testUser) {
	t.Helper()
	a, b := e.user(t), e.user(t)
	e.connect(t, a, b)
	return a, b
}

func (e *testEnv) direct(t *testing.T, a, b testUser) Conversation {
	t.Helper()
	rec := e.api.Post("/chat/conversations/direct", a.bearer, map[string]any{"userId": b.id})
	if rec.Code != http.StatusCreated && rec.Code != http.StatusOK {
		t.Fatalf("open direct: %d %s", rec.Code, rec.Body.String())
	}
	return decode[Conversation](t, rec, rec.Code)
}

func (e *testEnv) group(t *testing.T, owner testUser, name string, members ...testUser) Conversation {
	t.Helper()
	ids := make([]uuid.UUID, 0, len(members))
	for _, m := range members {
		ids = append(ids, m.id)
	}
	rec := e.api.Post("/chat/conversations/groups", owner.bearer, map[string]any{"name": name, "memberIds": ids})
	return decode[Conversation](t, rec, http.StatusCreated)
}

func keyHeader(key uuid.UUID) string {
	return "Idempotency-Key: " + key.String()
}

func (e *testEnv) rawSend(u testUser, conversationID uuid.UUID, key uuid.UUID, body map[string]any) *httptest.ResponseRecorder {
	return e.api.Post("/chat/conversations/"+conversationID.String()+"/messages", u.bearer, keyHeader(key), body)
}

func (e *testEnv) send(t *testing.T, u testUser, conversationID uuid.UUID, text string) Message {
	t.Helper()
	return decode[Message](t, e.rawSend(u, conversationID, uuid.New(), map[string]any{"body": text}), http.StatusCreated)
}

type messagePage struct {
	Items      []Message
	NextCursor *string
}

func (e *testEnv) history(t *testing.T, u testUser, conversationID uuid.UUID, query string) messagePage {
	t.Helper()
	rec := e.api.Get("/chat/conversations/"+conversationID.String()+"/messages?"+query, u.bearer)
	return decode[messagePage](t, rec, http.StatusOK)
}

func (e *testEnv) conversation(t *testing.T, u testUser, id uuid.UUID) Conversation {
	t.Helper()
	return decode[Conversation](t, e.api.Get("/chat/conversations/"+id.String(), u.bearer), http.StatusOK)
}

func (e *testEnv) pendingImage(t *testing.T, owner testUser) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := e.pool.Exec(context.Background(), `
		INSERT INTO media (id, owner_user_id, kind, object_key, content_type, size_bytes, status)
		VALUES ($1, $2, $3, $4, 'image/jpeg', 10, 'pending')`, id, owner.id, media.KindChat, "chat/"+id.String()+".jpg")
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	return id
}

func (e *testEnv) mediaStatus(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var status string
	err := e.pool.QueryRow(context.Background(), "SELECT status FROM media WHERE id = $1", id).Scan(&status)
	if err != nil {
		return "gone"
	}
	return status
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return v
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	body := decode[map[string]any](t, rec, wantStatus)
	if body["error"] != wantCode {
		t.Errorf(`expected {"error":%q}, got %v`, wantCode, body)
	}
}

func seqs(messages []Message) []int64 {
	out := make([]int64, 0, len(messages))
	for _, m := range messages {
		out = append(out, m.Seq)
	}
	return out
}

type fakePush struct {
	mu   sync.Mutex
	sent []push.SendArgs
}

func (f *fakePush) Notify(_ context.Context, args push.SendArgs) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, args)
	return nil
}

func (f *fakePush) Wake() {}

func (f *fakePush) notices() []push.SendArgs {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]push.SendArgs(nil), f.sent...)
}
