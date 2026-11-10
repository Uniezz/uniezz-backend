package faq

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
	pool  *pgxpool.Pool
	api   humatest.TestAPI
	users []uuid.UUID
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
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
	New(pool).Register(api)

	e := &testEnv{pool: pool, api: api}
	t.Cleanup(e.cleanup)
	return e
}

func (e *testEnv) cleanup() {
	ctx := context.Background()
	for _, query := range []string{
		"DELETE FROM faq_entries WHERE author_id = ANY($1)",
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

func (e *testEnv) user(t *testing.T) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx,
		"INSERT INTO users (university_id, email, first_name, last_name) VALUES ('umcs', $1, 'Jan', 'Kowalski') RETURNING id",
		"faq-"+uuid.NewString()+"@example.test").Scan(&id)
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

func (e *testEnv) insert(t *testing.T, author uuid.UUID, status string, createdAt time.Time) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `
		INSERT INTO faq_entries (author_id, question, answer, status, created_at)
		VALUES ($1, 'How do I sign in?', 'Use your university account.', $2, $3)`,
		author, status, createdAt)
	if err != nil {
		t.Fatalf("insert faq entry: %v", err)
	}
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
