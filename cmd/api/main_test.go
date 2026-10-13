package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/health"
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

func newSpecTestRouter(t *testing.T, env string) *chi.Mux {
	t.Helper()

	cfg := &config.Config{Env: env, BackendPublicURL: "https://api.test", AppWebURL: testWebAppURL}
	r, api := newRouter(cfg)
	health.Register(r, &atomic.Bool{}, nil)
	// No USOS credentials are needed in development; outside it Register would fail.
	cfg.Env = "development"
	if err := auth.Register(api, nil, cfg); err != nil {
		t.Fatalf("failed to register auth: %v", err)
	}
	return r
}

func get(r http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestOpenAPISpec(t *testing.T) {
	r := newSpecTestRouter(t, "development")

	for _, target := range []string{"/openapi.json", "/openapi.yaml", "/docs"} {
		if rec := get(r, target); rec.Code != http.StatusOK {
			t.Errorf("GET %s: expected 200, got %d", target, rec.Code)
		}
	}

	var spec struct {
		Paths map[string]map[string]struct {
			OperationID string                     `json:"operationId"`
			Tags        []string                   `json:"tags"`
			Responses   map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(get(r, "/openapi.json").Body.Bytes(), &spec); err != nil {
		t.Fatalf("spec is not valid JSON: %v", err)
	}

	for _, want := range []struct {
		path, method, operationID string
		statuses                  []string
	}{
		{"/auth/usos/start", "get", "startUsosLogin", []string{"302", "400"}},
		{"/auth/usos/callback", "get", "finishUsosLogin", []string{"302", "400"}},
		{"/auth/exchange", "post", "exchangeLoginCode", []string{"200", "400", "401"}},
	} {
		op, ok := spec.Paths[want.path][want.method]
		if !ok {
			t.Errorf("%s %s is missing from the spec", want.method, want.path)
			continue
		}
		if op.OperationID != want.operationID {
			t.Errorf("%s: expected operationId %q, got %q", want.path, want.operationID, op.OperationID)
		}
		if len(op.Tags) != 1 || op.Tags[0] != "auth" {
			t.Errorf("%s: expected tags [auth], got %v", want.path, op.Tags)
		}
		for _, status := range want.statuses {
			if _, ok := op.Responses[status]; !ok {
				t.Errorf("%s: expected a %s response in the spec", want.path, status)
			}
		}
	}

	for path := range spec.Paths {
		if strings.HasPrefix(path, "/healthz") {
			t.Errorf("%s should not be in the public spec", path)
		}
	}
	if rec := get(r, "/healthz/live"); rec.Code != http.StatusOK {
		t.Errorf("expected /healthz/live to still be served, got %d", rec.Code)
	}
}

func TestOpenAPISpec_NoDocsInProduction(t *testing.T) {
	r := newSpecTestRouter(t, "production")

	if rec := get(r, "/docs"); rec.Code != http.StatusNotFound {
		t.Errorf("GET /docs: expected 404 in production, got %d", rec.Code)
	}
	if rec := get(r, "/openapi.json"); rec.Code != http.StatusOK {
		t.Errorf("GET /openapi.json: expected 200 in production, got %d", rec.Code)
	}
}
