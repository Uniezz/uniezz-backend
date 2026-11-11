package connections

import (
	"context"
	"encoding/json"
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
	"github.com/Uniezz/uniezz-backend/internal/profile"
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

type testEnv struct {
	pool *pgxpool.Pool
	api  humatest.TestAPI
	tag  string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
	profiles, err := profile.New(pool, media.Module{}, jobs.NewRunner(pool, 1, time.Hour))
	if err != nil {
		t.Fatalf("profile.New failed: %v", err)
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
	profiles.Register(api)
	New(pool, profiles).Register(api)
	return &testEnv{pool: pool, api: api, tag: "test-" + uuid.NewString()}
}

type testUser struct {
	id     uuid.UUID
	bearer string
}

func (e *testEnv) user(t *testing.T) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx, `
		INSERT INTO users (university_id, email, first_name, last_name, bio, interests)
		VALUES ('umcs', $1, 'Jan', 'Kowalski', 'Bio', ARRAY[$2]) RETURNING id`,
		"connections-"+uuid.NewString()+"@example.test", e.tag).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM rate_limits WHERE key = $1", "connection-like:"+id.String())
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(database.New(e.pool)).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return testUser{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func (e *testEnv) openTo(t *testing.T, u testUser, modes ...string) {
	t.Helper()
	body := map[string]any{"birthDate": "2000-01-01", "openTo": modes}
	if rec := e.api.Patch("/connections/me", u.bearer, body); rec.Code != 204 {
		t.Fatalf("save preferences: %d %s", rec.Code, rec.Body.String())
	}
}

func (e *testEnv) discover(t *testing.T, u testUser, query string) []Card {
	t.Helper()
	rec := e.api.Get("/connections/discover?interests=shared&limit=20&"+query, u.bearer)
	return decode[struct{ Items []Card }](t, rec, 200).Items
}

func (e *testEnv) act(u testUser, candidateID uuid.UUID, mode, action string) *httptest.ResponseRecorder {
	return e.api.Put("/connections/candidates/"+candidateID.String(), u.bearer, map[string]any{"mode": mode, "action": action})
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
