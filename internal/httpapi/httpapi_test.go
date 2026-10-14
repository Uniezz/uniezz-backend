package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

// captureLog redirects the standard logger into a buffer for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prevOutput, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	})

	return &buf
}

func expectErrorBody(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, rec.Body.String())
	}
	if len(body) != 1 || body["error"] != wantCode {
		t.Errorf(`expected body {"error":%q}, got %s`, wantCode, rec.Body.String())
	}
}

type echoInput struct {
	Kind string `query:"kind" required:"true" enum:"a,b"`
	Body struct {
		Name string `json:"name" minLength:"1"`
	}
}

type echoOutput struct {
	Body struct {
		Name string `json:"name"`
	}
}

func newTestAPI(t *testing.T, handlerErr error) humatest.TestAPI {
	t.Helper()

	_, api := humatest.New(t, NewConfig(&config.Config{BackendPublicURL: "https://api.test"}))
	huma.Register(api, huma.Operation{
		OperationID: "echo",
		Method:      http.MethodPost,
		Path:        "/echo",
	}, func(_ context.Context, in *echoInput) (*echoOutput, error) {
		if handlerErr != nil {
			return nil, handlerErr
		}
		out := &echoOutput{}
		out.Body.Name = in.Body.Name
		return out, nil
	})
	return api
}

func TestNewConfig(t *testing.T) {
	c := NewConfig(&config.Config{Env: "development", BackendPublicURL: "https://api.test"})

	if len(c.Servers) != 1 || c.Servers[0].URL != "https://api.test" {
		t.Errorf("expected the server to be BACKEND_PUBLIC_URL, got %+v", c.Servers)
	}
	if s := c.Components.SecuritySchemes[BearerAuth]; s == nil || s.Type != "http" || s.Scheme != "bearer" {
		t.Errorf("expected an http bearer security scheme, got %+v", s)
	}
	if c.CreateHooks != nil {
		t.Error("expected no create hooks, so responses carry no $schema or Link header")
	}
	if c.DocsPath != "/docs" {
		t.Errorf("expected /docs outside production, got %q", c.DocsPath)
	}

	if c := NewConfig(&config.Config{Env: "production"}); c.DocsPath != "" {
		t.Errorf("expected no docs in production, got %q", c.DocsPath)
	}
}

func TestNewConfig_Version(t *testing.T) {
	prev := Version
	t.Cleanup(func() { Version = prev })

	if c := NewConfig(&config.Config{}); c.Info.Version != "dev" {
		t.Errorf("expected info.version %q by default, got %q", "dev", c.Info.Version)
	}

	Version = "6c4e8a0"
	if c := NewConfig(&config.Config{}); c.Info.Version != "6c4e8a0" {
		t.Errorf("expected info.version from Version, got %q", c.Info.Version)
	}
}

func TestNewConfig_NoServersWithoutPublicURL(t *testing.T) {
	// The committed spec is built without config, so it must not carry a bogus server.
	if c := NewConfig(&config.Config{}); len(c.Servers) != 0 {
		t.Errorf("expected no servers without BACKEND_PUBLIC_URL, got %+v", c.Servers)
	}
}

func TestSuccessHasNoSchemaLinks(t *testing.T) {
	rec := newTestAPI(t, nil).Post("/echo?kind=a", map[string]any{"name": "jan"})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "$schema") {
		t.Errorf("expected no $schema field, got %s", rec.Body.String())
	}
	if link := rec.Header().Get("Link"); link != "" {
		t.Errorf("expected no Link header, got %q", link)
	}
}

func TestValidationErrors(t *testing.T) {
	testCases := []struct {
		name   string
		target string
		body   any
	}{
		{name: "missing query param", target: "/echo", body: map[string]any{"name": "jan"}},
		{name: "value not in enum", target: "/echo?kind=c", body: map[string]any{"name": "jan"}},
		{name: "empty field", target: "/echo?kind=a", body: map[string]any{"name": ""}},
		{name: "unknown field", target: "/echo?kind=a", body: map[string]any{"name": "jan", "admin": true}},
		{name: "malformed JSON", target: "/echo?kind=a", body: strings.NewReader(`{"name":`)},
		{name: "no body", target: "/echo?kind=a", body: nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			api := newTestAPI(t, nil)

			var rec *httptest.ResponseRecorder
			if tc.body == nil {
				rec = api.Post(tc.target)
			} else {
				rec = api.Post(tc.target, tc.body, "Content-Type: application/json")
			}

			expectErrorBody(t, rec, http.StatusBadRequest, "invalid_input")
		})
	}
}

func TestHandlerError(t *testing.T) {
	testCases := []struct {
		name    string
		status  int
		code    string
		wantLog bool
	}{
		{name: "client error is not logged", status: http.StatusUnauthorized, code: "invalid_code", wantLog: false},
		{name: "server error is logged", status: http.StatusInternalServerError, code: "internal_error", wantLog: true},
		{name: "upstream error is logged", status: http.StatusBadGateway, code: "provider_unavailable", wantLog: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLog(t)
			api := newTestAPI(t, Error(tc.status, tc.code, errors.New("pq: connection to 10.0.0.5 refused")))

			rec := api.Post("/echo?kind=a", map[string]any{"name": "jan"})

			expectErrorBody(t, rec, tc.status, tc.code)
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("error detail leaked to the client: %s", rec.Body.String())
			}

			logged := logs.String()
			if tc.wantLog && !strings.Contains(logged, "10.0.0.5") {
				t.Errorf("expected the error detail to be logged, got %q", logged)
			}
			if !tc.wantLog && logged != "" {
				t.Errorf("expected no log for a client error, got %q", logged)
			}
		})
	}
}

func TestNewError(t *testing.T) {
	testCases := []struct {
		status     int
		wantStatus int
		wantCode   string
	}{
		{http.StatusBadRequest, http.StatusBadRequest, "invalid_input"},
		{http.StatusUnprocessableEntity, http.StatusBadRequest, "invalid_input"},
		{http.StatusNotFound, http.StatusNotFound, "not_found"},
		{http.StatusNotAcceptable, http.StatusNotAcceptable, "not_acceptable"},
		{http.StatusRequestEntityTooLarge, http.StatusRequestEntityTooLarge, "request_entity_too_large"},
		{http.StatusInternalServerError, http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range testCases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			captureLog(t)

			err := huma.NewError(tc.status, "some huma message", errors.New("detail"))

			if err.GetStatus() != tc.wantStatus || err.Error() != tc.wantCode {
				t.Errorf("NewError(%d) = %d %q, want %d %q", tc.status, err.GetStatus(), err.Error(), tc.wantStatus, tc.wantCode)
			}
		})
	}
}

func TestSpecErrorSchema(t *testing.T) {
	api := newTestAPI(t, nil)

	schema := api.OpenAPI().Components.Schemas.Map()["ErrorBody"]
	if schema == nil {
		t.Fatal("expected an ErrorBody schema in the spec")
	}
	if len(schema.Properties) != 1 || schema.Properties["error"] == nil {
		t.Errorf("expected the error schema to be {error: string}, got properties %v", schema.Properties)
	}
}
