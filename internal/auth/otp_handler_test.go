package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
)

type otpTestServer struct {
	api      humatest.TestAPI
	sender   *fakeOTPSender
	sessions *SessionRepository
	pool     *pgxpool.Pool
	email    string
}

func newOTPTestServer(t *testing.T) *otpTestServer {
	t.Helper()

	pool := setupTestDB(t)
	email := "otp-" + uuid.NewString() + "@student.kul.pl"

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM otp_requests WHERE email = $1", email)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE email = $1", email)
	})

	sender := &fakeOTPSender{}
	sessions := NewSessionRepository(pool)
	logins := newLogins(database.New(pool), NewUserRepository(pool), sessions, newLoginCodeRepository(pool), time.Hour)
	m := Module{
		otp: newOTPLogin(
			newOTPRequestRepository(pool),
			sender,
			ratelimit.New(pool, "otp-send-test", otpSendLimit, otpSendWindow),
			newOTPHasher([]byte("test-key")),
			nil,
		),
		logins: logins,
	}

	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{BackendPublicURL: "https://api.test"}))
	m.Register(api)

	return &otpTestServer{api: api, sender: sender, sessions: sessions, pool: pool, email: email}
}

func (s *otpTestServer) post(path string, body map[string]string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(body)
	return s.api.Do(http.MethodPost, path, strings.NewReader(string(b)), "Content-Type: application/json")
}

func (s *otpTestServer) requestCode(t *testing.T) {
	t.Helper()
	rec := s.post("/auth/otp/request", map[string]string{"university": "kul", "email": s.email})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("request: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

func (s *otpTestServer) verify(code string) *httptest.ResponseRecorder {
	return s.post("/auth/otp/verify", map[string]string{"university": "kul", "email": s.email, "code": code})
}

func wrongCode(right string) string {
	if right == "000000" {
		return "111111"
	}
	return "000000"
}

func TestOTPLogin_EndToEnd(t *testing.T) {
	s := newOTPTestServer(t)

	s.requestCode(t)
	if s.sender.to != s.email || len(s.sender.code) != 6 {
		t.Fatalf("expected a 6-digit code emailed to %s, got %q to %q", s.email, s.sender.code, s.sender.to)
	}

	rec := s.verify(s.sender.code)
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var issued IssuedSession
	if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
		t.Fatalf("verify: invalid JSON: %v", err)
	}
	hash, err := HashTokenString(issued.Token)
	if err != nil {
		t.Fatalf("token is not a valid session token: %v", err)
	}
	session, err := s.sessions.GetActiveSessionByHash(context.Background(), hash[:])
	if err != nil {
		t.Fatalf("session not found for the issued token: %v", err)
	}

	var userEmail, university string
	err = s.pool.QueryRow(context.Background(),
		"SELECT email, university_id FROM users WHERE id = $1", session.UserID).Scan(&userEmail, &university)
	if err != nil {
		t.Fatal(err)
	}
	if userEmail != s.email || university != "kul" {
		t.Errorf("expected the session to belong to %s at kul, got %s at %s", s.email, userEmail, university)
	}
}

func TestOTPLogin_SameUserOnNextLogin(t *testing.T) {
	s := newOTPTestServer(t)

	for range 2 {
		s.requestCode(t)
		if rec := s.verify(s.sender.code); rec.Code != http.StatusOK {
			t.Fatalf("verify: expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	var n int
	if err := s.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM users WHERE email = $1", s.email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected one user after two logins, got %d", n)
	}
}

func TestOTPLogin_Errors(t *testing.T) {
	t.Run("wrong code", func(t *testing.T) {
		s := newOTPTestServer(t)
		s.requestCode(t)

		expectJSONError(t, s.verify(wrongCode(s.sender.code)), http.StatusUnauthorized, "invalid_code")
	})

	t.Run("code is single use", func(t *testing.T) {
		s := newOTPTestServer(t)
		s.requestCode(t)

		if rec := s.verify(s.sender.code); rec.Code != http.StatusOK {
			t.Fatalf("first verify: expected 200, got %d", rec.Code)
		}
		expectJSONError(t, s.verify(s.sender.code), http.StatusUnauthorized, "invalid_code")
	})

	t.Run("fourth request in the window is rate limited", func(t *testing.T) {
		s := newOTPTestServer(t)
		for range 3 {
			s.requestCode(t)
		}

		rec := s.post("/auth/otp/request", map[string]string{"university": "kul", "email": s.email})
		expectJSONError(t, rec, http.StatusTooManyRequests, "rate_limited")
	})

	t.Run("foreign email domain", func(t *testing.T) {
		s := newOTPTestServer(t)

		rec := s.post("/auth/otp/request", map[string]string{"university": "kul", "email": "jan@gmail.com"})
		expectJSONError(t, rec, http.StatusBadRequest, "email_not_allowed")
	})

	t.Run("USOS university", func(t *testing.T) {
		s := newOTPTestServer(t)

		rec := s.post("/auth/otp/request", map[string]string{"university": "umcs", "email": "jan@umcs.pl"})
		expectJSONError(t, rec, http.StatusBadRequest, "wrong_auth_method")
	})

	t.Run("email failure", func(t *testing.T) {
		s := newOTPTestServer(t)
		s.sender.err = fmt.Errorf("resend down")

		rec := s.post("/auth/otp/request", map[string]string{"university": "kul", "email": s.email})
		expectJSONError(t, rec, http.StatusBadGateway, "provider_unavailable")
	})

	t.Run("malformed code is rejected before the database", func(t *testing.T) {
		s := newOTPTestServer(t)

		for _, code := range []string{"12345", "1234567", "abcdef", ""} {
			if rec := s.verify(code); rec.Code != http.StatusBadRequest {
				t.Errorf("code %q: expected 400, got %d", code, rec.Code)
			}
		}
	})
}
