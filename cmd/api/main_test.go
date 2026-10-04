package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

const testWebAppURL = "http://localhost:3000"

func newCORSTestRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(corsMiddleware(testWebAppURL))
	r.Post("/auth/exchange", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return r
}

func preflight(r http.Handler, origin, headers string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, "/auth/exchange", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", headers)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestCORS_PreflightFromWebApp(t *testing.T) {
	rec := preflight(newCORSTestRouter(), testWebAppURL, "authorization,content-type")

	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("expected a successful preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testWebAppURL {
		t.Errorf("expected Access-Control-Allow-Origin %q, got %q", testWebAppURL, got)
	}

	allowed := strings.ToLower(rec.Header().Get("Access-Control-Allow-Headers"))
	for _, h := range []string{"authorization", "content-type"} {
		if !strings.Contains(allowed, h) {
			t.Errorf("expected %q in Access-Control-Allow-Headers, got %q", h, allowed)
		}
	}
	if !strings.Contains(rec.Header().Get("Access-Control-Allow-Methods"), http.MethodPost) {
		t.Errorf("expected POST in Access-Control-Allow-Methods, got %q", rec.Header().Get("Access-Control-Allow-Methods"))
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "300" {
		t.Errorf("expected Access-Control-Max-Age 300, got %q", got)
	}
}

func TestCORS_RejectsOtherOrigins(t *testing.T) {
	for _, origin := range []string{
		"https://evil.example",
		"http://localhost:3001",
		"https://localhost:3000",
		testWebAppURL + "/",
	} {
		t.Run(origin, func(t *testing.T) {
			rec := preflight(newCORSTestRouter(), origin, "content-type")

			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("expected no Access-Control-Allow-Origin for %q, got %q", origin, got)
			}
		})
	}
}

func TestCORS_RejectsUnlistedHeaders(t *testing.T) {
	rec := preflight(newCORSTestRouter(), testWebAppURL, "x-custom-header")

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected a preflight asking for an unlisted header to be refused, got Access-Control-Allow-Origin %q", got)
	}
}

func TestCORS_ActualRequestFromWebApp(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/exchange", strings.NewReader(`{}`))
	req.Header.Set("Origin", testWebAppURL)
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	newCORSTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected the request to reach the handler, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != testWebAppURL {
		t.Errorf("expected Access-Control-Allow-Origin %q, got %q", testWebAppURL, got)
	}
}

func TestCORS_NeverAllowsCredentials(t *testing.T) {
	rec := preflight(newCORSTestRouter(), testWebAppURL, "authorization,content-type")

	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("expected no Access-Control-Allow-Credentials, got %q", got)
	}
}

func TestCORS_RequestWithoutOriginIsUntouched(t *testing.T) {
	// Mobile apps and server-to-server calls send no Origin header.
	req := httptest.NewRequest(http.MethodPost, "/auth/exchange", strings.NewReader(`{}`))

	rec := httptest.NewRecorder()
	newCORSTestRouter().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected the request to reach the handler, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no CORS headers without an Origin, got %q", got)
	}
}
