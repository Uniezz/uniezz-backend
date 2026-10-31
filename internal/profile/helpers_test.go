package profile

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
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

// fakeMedia keeps media rows in the real table, so the avatar foreign key holds,
// and skips object storage.
type fakeMedia struct {
	pool      *pgxpool.Pool
	deleted   []uuid.UUID
	imported  []string
	importErr error
	onImport  func()
}

func (f *fakeMedia) Claim(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error {
	result, err := f.pool.Exec(ctx,
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

func (f *fakeMedia) URL(_ context.Context, mediaID uuid.UUID) (string, error) {
	return "https://media.test/" + mediaID.String(), nil
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
	f.deleted = append(f.deleted, mediaID)
	_, err := f.pool.Exec(ctx, "DELETE FROM media WHERE id = $1 AND status = 'deleting'", mediaID)
	return err
}

func (f *fakeMedia) Import(ctx context.Context, ownerUserID uuid.UUID, kind media.Kind, sourceURL string) (uuid.UUID, error) {
	f.imported = append(f.imported, sourceURL)
	if f.importErr != nil {
		return uuid.Nil, f.importErr
	}
	id := insertMedia(ctx, f.pool, ownerUserID, "ready")
	if f.onImport != nil {
		f.onImport()
	}
	return id, nil
}

func insertMedia(ctx context.Context, pool *pgxpool.Pool, ownerID uuid.UUID, status string) uuid.UUID {
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO media (owner_user_id, kind, object_key, content_type, size_bytes, status)
		VALUES ($1, 'avatar', 'avatars/' || gen_random_uuid() || '.jpg', 'image/jpeg', 10, $2)
		RETURNING id`, ownerID, status).Scan(&id)
	if err != nil {
		panic(err)
	}
	return id
}

func mediaStatus(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()

	var s string
	err := pool.QueryRow(context.Background(), "SELECT status FROM media WHERE id = $1", id).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return "gone"
	}
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
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
	fm := &fakeMedia{pool: pool}
	runner := jobs.NewRunner(pool, 1)
	m, err := New(pool, fm, runner)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	authModule, err := auth.New(pool, &config.Config{UsosConfigs: map[string]config.UsosCredentials{
		"umcs":  {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
		"umlub": {BaseURL: "https://usos.test", ConsumerKey: "key", ConsumerSecret: "secret"},
	}}, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	m.Register(api)

	return &testEnv{pool: pool, api: api, media: fm, module: m}
}

type testUser struct {
	id     uuid.UUID
	bearer string
}

func (e *testEnv) newUser(t *testing.T, university, email string) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx,
		"INSERT INTO users (university_id, email, first_name, last_name) VALUES ($1, $2, 'Jan', 'Kowalski') RETURNING id",
		university, email).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM jobs WHERE payload->>'userId' = $1", id.String())
	})

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(e.pool).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return testUser{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func (e *testEnv) otpUser(t *testing.T) testUser {
	return e.newUser(t, "kul", "profile-"+uuid.NewString()+"@student.kul.pl")
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
