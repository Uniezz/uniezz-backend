package push

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

type fakeSender struct {
	mu       sync.Mutex
	messages []expoMessage
	invalid  map[string]bool
}

func (f *fakeSender) send(_ context.Context, messages []expoMessage) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, messages...)
	var invalid []string
	for _, msg := range messages {
		if f.invalid[msg.To] {
			invalid = append(invalid, msg.To)
		}
	}
	return invalid, nil
}

func (f *fakeSender) to(token string) []expoMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []expoMessage
	for _, msg := range f.messages {
		if msg.To == token {
			out = append(out, msg)
		}
	}
	return out
}

type testEnv struct {
	pool   *pgxpool.Pool
	api    humatest.TestAPI
	expo   *fakeSender
	module Module
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
	module, err := New(pool, "", jobs.NewRunner(pool, 1, time.Hour))
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	expo := &fakeSender{invalid: map[string]bool{}}
	module.expo = expo

	authModule, err := auth.New(pool, &config.Config{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("auth.New failed: %v", err)
	}
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(authModule.Guard(api))
	module.Register(api)
	return &testEnv{pool: pool, api: api, expo: expo, module: module}
}

type testUser struct {
	id        uuid.UUID
	sessionID uuid.UUID
	bearer    string
}

func (e *testEnv) user(t *testing.T, language string) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx, `
		INSERT INTO users (university_id, email, first_name, last_name, language)
		VALUES ('umcs', $1, 'Jan', 'Kowalski', $2) RETURNING id`,
		"push-"+uuid.NewString()+"@example.test", language).Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	return e.session(t, id)
}

func (e *testEnv) session(t *testing.T, userID uuid.UUID) testUser {
	t.Helper()
	token := auth.GenerateSessionToken()
	hash := token.Hash()
	session, err := auth.NewSessionRepository(database.New(e.pool)).CreateSession(context.Background(), userID, hash[:], time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return testUser{id: userID, sessionID: session.ID, bearer: "Authorization: Bearer " + token.String()}
}

func newToken() string {
	return "ExponentPushToken[" + uuid.NewString()[:22] + "]"
}

func (e *testEnv) register(t *testing.T, u testUser, token string) {
	t.Helper()
	rec := e.api.Put("/push/tokens/me", u.bearer, map[string]any{"token": token})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("register token: %d %s", rec.Code, rec.Body.String())
	}
}

func (e *testEnv) tokensOf(t *testing.T, userID uuid.UUID) map[string]uuid.UUID {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), "SELECT token, session_id FROM push_tokens WHERE user_id = $1", userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]uuid.UUID{}
	for rows.Next() {
		var token string
		var sessionID uuid.UUID
		if err := rows.Scan(&token, &sessionID); err != nil {
			t.Fatal(err)
		}
		out[token] = sessionID
	}
	return out
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) T {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func expectError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("expected %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error != wantCode {
		t.Errorf("error = %q (%v), want %q", body.Error, err, wantCode)
	}
}
