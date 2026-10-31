package moderation

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
	"github.com/Uniezz/uniezz-backend/internal/feed"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
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

// fakeTargets stands in for feed and profile: every known id belongs to the
// given author.
type fakeTargets struct {
	authors map[uuid.UUID]uuid.UUID
}

func (f fakeTargets) ReportTarget(_ context.Context, _ string, id uuid.UUID) (uuid.UUID, map[string]any, error) {
	author, ok := f.authors[id]
	if !ok {
		return uuid.Nil, nil, feed.ErrNotFound
	}
	return author, map[string]any{"content": "reported text"}, nil
}

type fakeProfiles struct {
	users map[uuid.UUID]bool
}

func (f fakeProfiles) ReportTarget(_ context.Context, id uuid.UUID) (map[string]any, error) {
	if !f.users[id] {
		return nil, profile.ErrNotFound
	}
	return map[string]any{"firstName": "Jan"}, nil
}

type testEnv struct {
	pool    *pgxpool.Pool
	api     humatest.TestAPI
	targets fakeTargets
	users   fakeProfiles
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	pool := setupTestDB(t)
	e := &testEnv{pool: pool, targets: fakeTargets{authors: map[uuid.UUID]uuid.UUID{}}, users: fakeProfiles{users: map[uuid.UUID]bool{}}}
	m := New(pool, e.targets, e.users)

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
	e.api = api
	return e
}

type testUser struct {
	id     uuid.UUID
	bearer string
}

func (e *testEnv) user(t *testing.T) testUser {
	t.Helper()
	ctx := context.Background()

	var id uuid.UUID
	err := e.pool.QueryRow(ctx, "INSERT INTO users (university_id, email) VALUES ('umcs', $1) RETURNING id",
		"moderation-"+uuid.NewString()+"@example.test").Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM reports WHERE reporter_id = $1 OR target_author_id = $1", id)
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM rate_limits WHERE key = $1", "report:"+id.String())
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id)
	})
	e.users.users[id] = true

	token := auth.GenerateSessionToken()
	hash := token.Hash()
	if _, err := auth.NewSessionRepository(database.New(e.pool)).CreateSession(ctx, id, hash[:], time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return testUser{id: id, bearer: "Authorization: Bearer " + token.String()}
}

func (e *testEnv) contentBy(author testUser) uuid.UUID {
	id := uuid.New()
	e.targets.authors[id] = author.id
	return id
}

func report(targetType string, id uuid.UUID) map[string]any {
	return map[string]any{"targetType": targetType, "targetId": id, "reason": "spam", "details": " <b>Buy</b> now "}
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
	body := decode[map[string]string](t, rec, wantStatus)
	if body["error"] != wantCode {
		t.Errorf(`expected {"error":%q}, got %v`, wantCode, body)
	}
}

func TestCreateReport(t *testing.T) {
	e := newTestEnv(t)
	reporter := e.user(t)
	author := e.user(t)
	post := e.contentBy(author)

	got := decode[Report](t, e.api.Post("/reports", reporter.bearer, report("post", post)), 201)
	if got.Status != "pending" || got.TargetID != post || got.Details == nil || *got.Details != "Buy now" {
		t.Errorf("report = %+v", got)
	}

	var snapshot map[string]any
	var targetAuthor uuid.UUID
	err := e.pool.QueryRow(context.Background(), "SELECT snapshot, target_author_id FROM reports WHERE id = $1", got.ID).Scan(&snapshot, &targetAuthor)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot["content"] != "reported text" || targetAuthor != author.id {
		t.Errorf("stored snapshot = %v author = %s", snapshot, targetAuthor)
	}

	expectError(t, e.api.Post("/reports", reporter.bearer, report("post", post)), 409, "already_reported")
	expectError(t, e.api.Post("/reports", author.bearer, report("post", post)), 400, "self_report")
	expectError(t, e.api.Post("/reports", reporter.bearer, report("user", reporter.id)), 400, "self_report")
	expectError(t, e.api.Post("/reports", reporter.bearer, report("comment", uuid.New())), 404, "not_found")
	expectError(t, e.api.Post("/reports", reporter.bearer, report("user", uuid.New())), 404, "not_found")
	expectError(t, e.api.Post("/reports", reporter.bearer, map[string]any{"targetType": "post", "targetId": post, "reason": "boring"}), 400, "invalid_input")
	decode[Report](t, e.api.Post("/reports", reporter.bearer, report("user", author.id)), 201)
	expectError(t, e.api.Post("/reports", report("user", author.id)), 401, "unauthorized")
}

// awayFromHourBoundary keeps a limit test inside one fixed rate-limit window.
func awayFromHourBoundary(t *testing.T) {
	t.Helper()
	if left := time.Until(time.Now().Truncate(time.Hour).Add(time.Hour)); left < 10*time.Second {
		time.Sleep(left + time.Second)
	}
}

func TestReportLimits(t *testing.T) {
	awayFromHourBoundary(t)
	e := newTestEnv(t)
	author := e.user(t)

	t.Run("concurrent reports from one user pass the limit five times", func(t *testing.T) {
		reporter := e.user(t)
		targets := make([]uuid.UUID, 10)
		for i := range targets {
			targets[i] = e.contentBy(author)
		}

		codes := make(chan int, len(targets))
		var wg sync.WaitGroup
		for _, id := range targets {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes <- e.api.Post("/reports", reporter.bearer, report("post", id)).Code
			}()
		}
		wg.Wait()
		close(codes)

		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[201] != reportLimit || counts[429] != len(targets)-reportLimit {
			t.Errorf("statuses = %v, want %d created and the rest 429", counts, reportLimit)
		}
	})

	t.Run("concurrent duplicates create one report", func(t *testing.T) {
		reporter := e.user(t)
		post := e.contentBy(author)

		codes := make(chan int, 4)
		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes <- e.api.Post("/reports", reporter.bearer, report("post", post)).Code
			}()
		}
		wg.Wait()
		close(codes)

		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[201] != 1 || counts[409] != 3 {
			t.Errorf("statuses = %v, want one 201 and three 409", counts)
		}
	})

	t.Run("concurrent duplicates do not use the quota", func(t *testing.T) {
		reporter := e.user(t)
		post := e.contentBy(author)
		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				e.api.Post("/reports", reporter.bearer, report("post", post))
			}()
		}
		wg.Wait()
		for range reportLimit - 1 {
			decode[Report](t, e.api.Post("/reports", reporter.bearer, report("post", e.contentBy(author))), 201)
		}
	})

	t.Run("a rejected duplicate does not use the quota", func(t *testing.T) {
		reporter := e.user(t)
		post := e.contentBy(author)
		decode[Report](t, e.api.Post("/reports", reporter.bearer, report("post", post)), 201)
		for range 3 {
			expectError(t, e.api.Post("/reports", reporter.bearer, report("post", post)), 409, "already_reported")
		}
		for range reportLimit - 1 {
			decode[Report](t, e.api.Post("/reports", reporter.bearer, report("post", e.contentBy(author))), 201)
		}
	})
}
