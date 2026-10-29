package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
)

type fakeSessions struct {
	session  *Session
	err      error
	extended *time.Time
}

func (f *fakeSessions) GetActiveSessionByHash(_ context.Context, hash []byte) (*Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.session == nil || string(f.session.TokenHash) != string(hash) {
		return nil, ErrSessionNotFound
	}
	return f.session, nil
}

func (f *fakeSessions) ExtendSession(_ context.Context, _ uuid.UUID, expiresAt time.Time) error {
	f.extended = &expiresAt
	return nil
}

var guardNow = time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)

func newGuardTestAPI(t *testing.T, sessions sessionStore) humatest.TestAPI {
	t.Helper()

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	m := Module{guard: &guard{sessions: sessions, sessionTTL: 30 * 24 * time.Hour, now: func() time.Time { return guardNow }}}
	api.UseMiddleware(m.Guard(api))

	type whoami struct {
		Body struct {
			UserID string `json:"userId"`
		}
	}
	huma.Register(api, Protected(huma.Operation{Method: http.MethodGet, Path: "/protected"}),
		func(ctx context.Context, _ *struct{}) (*whoami, error) {
			out := &whoami{}
			id, ok := UserIDFrom(ctx)
			if !ok {
				return nil, errors.New("no user in context")
			}
			out.Body.UserID = id.String()
			return out, nil
		})
	huma.Register(api, huma.Operation{Method: http.MethodGet, Path: "/public"},
		func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			if _, ok := UserIDFrom(ctx); ok {
				return nil, errors.New("public route must not get a user")
			}
			return nil, nil
		})
	return api
}

func validSession(expiresIn time.Duration) (string, *Session) {
	token := GenerateSessionToken()
	hash := token.Hash()
	return token.String(), &Session{ID: uuid.New(), UserID: uuid.New(), TokenHash: hash[:], ExpiresAt: guardNow.Add(expiresIn)}
}

func TestGuard(t *testing.T) {
	token, session := validSession(20 * 24 * time.Hour)

	t.Run("a valid bearer token reaches the handler with its user", func(t *testing.T) {
		api := newGuardTestAPI(t, &fakeSessions{session: session})

		rec := api.Get("/protected", "Authorization: Bearer "+token)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if want := `{"userId":"` + session.UserID.String() + `"}`; rec.Body.String() != want+"\n" && rec.Body.String() != want {
			t.Errorf("expected body %s, got %s", want, rec.Body.String())
		}
	})

	for name, header := range map[string]string{
		"missing header":         "",
		"wrong scheme":           "Basic " + token,
		"token that won't parse": "Bearer not-a-token",
		"unknown or expired":     "Bearer " + GenerateSessionToken().String(),
		"lowercase scheme":       "bearer " + token,
	} {
		t.Run(name+" gets the same 401", func(t *testing.T) {
			api := newGuardTestAPI(t, &fakeSessions{session: session})

			var headers []any
			if header != "" {
				headers = append(headers, "Authorization: "+header)
			}
			expectJSONError(t, api.Get("/protected", headers...), http.StatusUnauthorized, "unauthorized")
		})
	}

	t.Run("a database failure is a 500, not a 401", func(t *testing.T) {
		api := newGuardTestAPI(t, &fakeSessions{err: errors.New("connection refused")})

		expectJSONError(t, api.Get("/protected", "Authorization: Bearer "+token), http.StatusInternalServerError, "internal_error")
	})

	t.Run("public operations run without a token", func(t *testing.T) {
		api := newGuardTestAPI(t, &fakeSessions{})

		if rec := api.Get("/public"); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	})
}

func TestGuard_SlidingExpiry(t *testing.T) {
	const ttl = 30 * 24 * time.Hour

	for _, tc := range []struct {
		name       string
		expiresIn  time.Duration
		wantExtend bool
	}{
		{"more than half the TTL left: no write", ttl/2 + time.Minute, false},
		{"less than half the TTL left: pushed to now + TTL", ttl/2 - time.Minute, true},
		{"about to expire: pushed to now + TTL", time.Minute, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token, session := validSession(tc.expiresIn)
			sessions := &fakeSessions{session: session}
			g := &guard{sessions: sessions, sessionTTL: ttl, now: func() time.Time { return guardNow }}

			if _, err := g.authenticate(context.Background(), "Bearer "+token); err != nil {
				t.Fatalf("authenticate failed: %v", err)
			}

			if !tc.wantExtend {
				if sessions.extended != nil {
					t.Errorf("expected no extension, got %v", sessions.extended)
				}
				return
			}
			if sessions.extended == nil || !sessions.extended.Equal(guardNow.Add(ttl)) {
				t.Errorf("expected expiry %v, got %v", guardNow.Add(ttl), sessions.extended)
			}
		})
	}
}

func TestSessionRepository_ExtendSession(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	repo := NewSessionRepository(pool)
	user := upsertTestUser(t, pool, NewUserRepository(pool), Identity{UniversityID: UniversityKUL, Email: testEmail()})

	token := GenerateSessionToken()
	hash := token.Hash()
	s, err := repo.CreateSession(ctx, user.ID, hash[:], time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	later := time.Now().Add(48 * time.Hour).Truncate(time.Microsecond)
	if err := repo.ExtendSession(ctx, s.ID, later); err != nil {
		t.Fatalf("ExtendSession failed: %v", err)
	}
	if err := repo.ExtendSession(ctx, s.ID, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("ExtendSession failed: %v", err)
	}

	got, err := repo.GetActiveSessionByHash(ctx, hash[:])
	if err != nil {
		t.Fatalf("GetActiveSessionByHash failed: %v", err)
	}
	if !got.ExpiresAt.Equal(later) {
		t.Errorf("expected expiry %v, got %v (an earlier time must never shorten a session)", later, got.ExpiresAt)
	}
}
