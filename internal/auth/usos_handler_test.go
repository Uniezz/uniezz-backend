package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Uniezz/uniezz-backend/internal/usos"
)

const (
	testWebRedirectURL    = "https://app.test/auth/done"
	testMobileRedirectURL = "uniezz://auth/callback"
)

// authTestServer runs the USOS and session handlers against the real
// database, with a fake USOS installation for UMCS.
type authTestServer struct {
	router   *chi.Mux
	usos     *fakeUSOS
	sessions *SessionRepository
	pool     *pgxpool.Pool
}

func newAuthTestServer(t *testing.T) *authTestServer {
	t.Helper()

	pool := setupTestDB(t)

	requestToken := "test-" + uuid.NewString()
	student := activeStudent()
	student.ID = testUsosUserID()

	client := &fakeUSOS{
		rt:           usos.RequestToken{Token: requestToken, Secret: "secret"},
		authorizeURL: "https://usos.test/services/oauth/authorize?oauth_token=" + requestToken,
		user:         student,
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM oauth_states WHERE request_token = $1", requestToken)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE usos_user_id = $1", student.ID)
	})

	sessions := NewSessionRepository(pool)
	logins := newLogins(NewUserRepository(pool), sessions, newLoginCodeRepository(pool), time.Hour)

	usosH := &usosHandler{
		usos:              newUsosLogin(map[UniversityID]usosClient{UniversityUMCS: client}, newUsosStateRepository(pool)),
		logins:            logins,
		webRedirectURL:    testWebRedirectURL,
		mobileRedirectURL: testMobileRedirectURL,
	}
	sessionH := &sessionHandler{logins: logins}

	// Same routes as Register.
	r := chi.NewRouter()
	r.Route("/auth", func(r chi.Router) {
		r.Get("/usos/start", usosH.start)
		r.Get("/usos/callback", usosH.callback)
		r.Post("/exchange", sessionH.exchange)
	})

	return &authTestServer{router: r, usos: client, sessions: sessions, pool: pool}
}

func (s *authTestServer) do(t *testing.T, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()

	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

func (s *authTestServer) start(t *testing.T, platform Platform) {
	t.Helper()

	rec := s.do(t, http.MethodGet, "/auth/usos/start?university=umcs&platform="+string(platform), "")
	if rec.Code != http.StatusFound {
		t.Fatalf("start: expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
}

func (s *authTestServer) callback(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()

	q := url.Values{"oauth_token": {s.usos.rt.Token}, "oauth_verifier": {"verifier-1"}}
	return s.do(t, http.MethodGet, "/auth/usos/callback?"+q.Encode(), "")
}

// redirectParams checks a 302 to wantBase and returns its query parameters.
func redirectParams(t *testing.T, rec *httptest.ResponseRecorder, wantBase string) url.Values {
	t.Helper()

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	location := rec.Header().Get("Location")
	base, rawQuery, _ := strings.Cut(location, "?")
	if base != wantBase {
		t.Fatalf("expected redirect to %q, got %q", wantBase, location)
	}

	params, err := url.ParseQuery(rawQuery)
	if err != nil {
		t.Fatalf("redirect has a malformed query %q: %v", rawQuery, err)
	}
	return params
}

func expectJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %v\nbody: %s", err, rec.Body.String())
	}
	if body["error"] != wantCode {
		t.Errorf(`expected {"error":%q}, got %s`, wantCode, rec.Body.String())
	}
}

func usersWithUsosID(t *testing.T, pool *pgxpool.Pool, usosUserID string) int {
	t.Helper()

	var n int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM users WHERE usos_user_id = $1", usosUserID).Scan(&n)
	if err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	return n
}

func TestUsosLogin_EndToEnd(t *testing.T) {
	for _, tc := range []struct {
		platform     Platform
		redirectBase string
	}{
		{PlatformWeb, testWebRedirectURL},
		{PlatformMobile, testMobileRedirectURL},
	} {
		t.Run(string(tc.platform), func(t *testing.T) {
			s := newAuthTestServer(t)

			// 1. The app sends the browser to start, which redirects to USOS.
			rec := s.do(t, http.MethodGet, "/auth/usos/start?university=umcs&platform="+string(tc.platform), "")
			if rec.Code != http.StatusFound {
				t.Fatalf("start: expected 302, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Location"); got != s.usos.authorizeURL {
				t.Fatalf("start: expected redirect to USOS %q, got %q", s.usos.authorizeURL, got)
			}

			// 2. USOS sends the browser to the callback, which redirects to the app with a code.
			params := redirectParams(t, s.callback(t), tc.redirectBase)
			code := params.Get("code")
			if code == "" || params.Get("error") != "" {
				t.Fatalf("callback: expected ?code= and no error, got %v", params)
			}

			// 3. The app trades the code for a session token.
			rec = s.do(t, http.MethodPost, "/auth/exchange", `{"code":"`+code+`"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("exchange: expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("exchange: expected JSON, got Content-Type %q", ct)
			}

			var issued IssuedSession
			if err := json.Unmarshal(rec.Body.Bytes(), &issued); err != nil {
				t.Fatalf("exchange: invalid JSON: %v", err)
			}
			if issued.Token == "" || issued.ExpiresAt.IsZero() {
				t.Fatalf("exchange: expected token and expiresAt, got %s", rec.Body.String())
			}

			// 4. The token belongs to the user USOS vouched for.
			hash, err := HashTokenString(issued.Token)
			if err != nil {
				t.Fatalf("token is not a valid session token: %v", err)
			}
			session, err := s.sessions.GetActiveSessionByHash(context.Background(), hash[:])
			if err != nil {
				t.Fatalf("token does not resolve to an active session: %v", err)
			}

			var usosUserID string
			err = s.pool.QueryRow(context.Background(),
				"SELECT usos_user_id FROM users WHERE id = $1", session.UserID).Scan(&usosUserID)
			if err != nil {
				t.Fatalf("failed to load session user: %v", err)
			}
			if usosUserID != s.usos.user.ID {
				t.Errorf("expected the session to belong to usos user %q, got %q", s.usos.user.ID, usosUserID)
			}

			// 5. The code is single-use.
			rec = s.do(t, http.MethodPost, "/auth/exchange", `{"code":"`+code+`"}`)
			expectJSONError(t, rec, http.StatusUnauthorized, "invalid_code")
		})
	}
}

func TestUsosHandler_Start(t *testing.T) {
	t.Run("missing or unknown platform is a JSON 400", func(t *testing.T) {
		s := newAuthTestServer(t)

		for _, target := range []string{
			"/auth/usos/start?university=umcs",
			"/auth/usos/start?university=umcs&platform=desktop",
		} {
			expectJSONError(t, s.do(t, http.MethodGet, target, ""), http.StatusBadRequest, "invalid_input")
		}
		if s.usos.beginCalls != 0 {
			t.Error("expected USOS not to be called")
		}
	})

	t.Run("OTP university redirects back with wrong_auth_method", func(t *testing.T) {
		s := newAuthTestServer(t)

		rec := s.do(t, http.MethodGet, "/auth/usos/start?university=kul&platform=web", "")

		params := redirectParams(t, rec, testWebRedirectURL)
		if params.Get("error") != "wrong_auth_method" {
			t.Errorf("expected ?error=wrong_auth_method, got %v", params)
		}
	})

	t.Run("unknown university redirects back with unknown_university", func(t *testing.T) {
		s := newAuthTestServer(t)

		rec := s.do(t, http.MethodGet, "/auth/usos/start?university=nope&platform=mobile", "")

		params := redirectParams(t, rec, testMobileRedirectURL)
		if params.Get("error") != "unknown_university" {
			t.Errorf("expected ?error=unknown_university, got %v", params)
		}
	})

	t.Run("USOS failure redirects back with provider_unavailable", func(t *testing.T) {
		s := newAuthTestServer(t)
		s.usos.beginErr = errors.New("oauth1: invalid status 503")

		rec := s.do(t, http.MethodGet, "/auth/usos/start?university=umcs&platform=web", "")

		params := redirectParams(t, rec, testWebRedirectURL)
		if params.Get("error") != "provider_unavailable" {
			t.Errorf("expected ?error=provider_unavailable, got %v", params)
		}
		if strings.Contains(rec.Header().Get("Location"), "503") {
			t.Errorf("error detail leaked into the redirect: %s", rec.Header().Get("Location"))
		}
	})
}

func TestUsosHandler_Callback(t *testing.T) {
	t.Run("non-student is sent back with not_student and no account is created", func(t *testing.T) {
		s := newAuthTestServer(t)
		s.usos.user.StudentStatus = ptr(usos.StatusInactiveStudent)
		s.start(t, PlatformMobile)

		params := redirectParams(t, s.callback(t), testMobileRedirectURL)

		if params.Get("error") != "not_student" || params.Get("code") != "" {
			t.Errorf("expected ?error=not_student and no code, got %v", params)
		}
		if n := usersWithUsosID(t, s.pool, s.usos.user.ID); n != 0 {
			t.Errorf("expected no user to be created, found %d", n)
		}
	})

	t.Run("USOS failure is sent back with provider_unavailable", func(t *testing.T) {
		s := newAuthTestServer(t)
		s.usos.finishErr = errors.New("usos api returned status: 500")
		s.start(t, PlatformWeb)

		params := redirectParams(t, s.callback(t), testWebRedirectURL)

		if params.Get("error") != "provider_unavailable" {
			t.Errorf("expected ?error=provider_unavailable, got %v", params)
		}
	})

	t.Run("unknown state is a JSON 400, as there is no app to return to", func(t *testing.T) {
		s := newAuthTestServer(t)

		rec := s.callback(t) // start was never called

		expectJSONError(t, rec, http.StatusBadRequest, "login_expired")
		if s.usos.finishCalls != 0 {
			t.Error("expected USOS not to be called")
		}
	})

	t.Run("replayed callback is a JSON 400", func(t *testing.T) {
		s := newAuthTestServer(t)
		s.start(t, PlatformWeb)

		redirectParams(t, s.callback(t), testWebRedirectURL)

		expectJSONError(t, s.callback(t), http.StatusBadRequest, "login_expired")
	})

	t.Run("missing oauth parameters are a JSON 400", func(t *testing.T) {
		s := newAuthTestServer(t)

		for _, target := range []string{
			"/auth/usos/callback",
			"/auth/usos/callback?oauth_token=abc",
			"/auth/usos/callback?oauth_verifier=abc",
		} {
			expectJSONError(t, s.do(t, http.MethodGet, target, ""), http.StatusBadRequest, "invalid_input")
		}
	})

	t.Run("repeated login reuses the account", func(t *testing.T) {
		s := newAuthTestServer(t)

		for i := 0; i < 2; i++ {
			s.start(t, PlatformWeb)
			params := redirectParams(t, s.callback(t), testWebRedirectURL)
			if params.Get("code") == "" {
				t.Fatalf("login %d: expected a code, got %v", i+1, params)
			}
		}

		if n := usersWithUsosID(t, s.pool, s.usos.user.ID); n != 1 {
			t.Errorf("expected exactly one user after two logins, found %d", n)
		}
	})
}

func TestSessionHandler_Exchange(t *testing.T) {
	s := newAuthTestServer(t)

	testCases := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{name: "malformed JSON", body: `{"code":`, wantStatus: http.StatusBadRequest, wantCode: "invalid_input"},
		{name: "unknown field", body: `{"code":"x","token":"y"}`, wantStatus: http.StatusBadRequest, wantCode: "invalid_input"},
		{name: "empty code", body: `{"code":""}`, wantStatus: http.StatusUnauthorized, wantCode: "invalid_code"},
		{name: "garbage code", body: `{"code":"not-a-code"}`, wantStatus: http.StatusUnauthorized, wantCode: "invalid_code"},
		{name: "well-formed unknown code", body: `{"code":"` + GenerateSessionToken().String() + `"}`, wantStatus: http.StatusUnauthorized, wantCode: "invalid_code"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rec := s.do(t, http.MethodPost, "/auth/exchange", tc.body)
			expectJSONError(t, rec, tc.wantStatus, tc.wantCode)
		})
	}
}
