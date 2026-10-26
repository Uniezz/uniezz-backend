package telemetry

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const testUserID = "u-7f3a9c"

func newTestRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/users/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/status/{code}", func(w http.ResponseWriter, r *http.Request) {
		var code int
		_, _ = fmt.Sscan(chi.URLParam(r, "code"), &code)
		w.WriteHeader(code)
	})
	r.Get("/silent", func(http.ResponseWriter, *http.Request) {})
	r.Get("/panic", func(http.ResponseWriter, *http.Request) {
		panic("boom: oauth_verifier=" + secretVerifier)
	})
	r.Get("/fail", func(w http.ResponseWriter, r *http.Request) {
		ReportError(r.Context(), "load profile", fmt.Errorf("query failed: %w", errors.New("Bearer "+secretSessionToken)))
		w.WriteHeader(http.StatusInternalServerError)
	})
	r.Get("/healthz/ready", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})
	return r
}

func serve(h http.Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = "203.0.113.7:51234"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func spanAttr(attrs []attribute.KeyValue, key attribute.Key) string {
	for _, kv := range attrs {
		if kv.Key == key {
			return kv.Value.String()
		}
	}
	return ""
}

func TestMiddleware_LogsOneRecordPerRequest(t *testing.T) {
	tt := setupTestTelemetry(t)
	r := newTestRouter()

	rec := serve(r, "/users/"+testUserID+"?oauth_verifier="+secretVerifier, map[string]string{
		"Authorization": "Bearer " + secretSessionToken,
		"Cookie":        "session=" + secretSessionToken,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	logs := tt.stdout.withMsg(t, "request")
	if len(logs) != 1 {
		t.Fatalf("expected one request record, got %d: %s", len(logs), tt.stdout.String())
	}
	got := logs[0]

	spans := tt.spans.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	span := spans[0]

	for key, want := range map[string]any{
		"level":      "INFO",
		"method":     "GET",
		"route":      "/users/{id}",
		"status":     float64(200),
		"client_ip":  "203.0.113.7",
		"request_id": rec.Header().Get(RequestIDHeader),
		"trace_id":   span.SpanContext.TraceID().String(),
		"span_id":    span.SpanContext.SpanID().String(),
		"service":    testResource.Service,
		"env":        testResource.Env,
		"version":    testResource.Version,
	} {
		if got[key] != want {
			t.Errorf("%s: expected %v, got %v", key, want, got[key])
		}
	}
	if _, ok := got["duration_ms"].(float64); !ok {
		t.Errorf("expected a numeric duration_ms, got %v", got["duration_ms"])
	}
	if strings.Contains(tt.stdout.String(), testUserID) {
		t.Errorf("expected the raw path to stay out of the logs, got %s", tt.stdout.String())
	}

	exported := tt.logs.all()
	if len(exported) != 1 {
		t.Fatalf("expected one exported record, got %d", len(exported))
	}
	if exported[0].TraceID() != span.SpanContext.TraceID() {
		t.Errorf("expected the exported record to link to trace %s, got %s", span.SpanContext.TraceID(), exported[0].TraceID())
	}

	tt.expectNoSecrets(t)
}

func TestMiddleware_LevelByStatus(t *testing.T) {
	for _, tc := range []struct {
		target, level string
	}{
		{"/status/200", "INFO"},
		{"/status/302", "INFO"},
		{"/status/404", "WARN"},
		{"/status/429", "WARN"},
		{"/status/500", "ERROR"},
		{"/status/502", "ERROR"},
		{"/silent", "INFO"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			tt := setupTestTelemetry(t)
			serve(newTestRouter(), tc.target, nil)

			logs := tt.stdout.withMsg(t, "request")
			if len(logs) != 1 {
				t.Fatalf("expected one request record, got %d", len(logs))
			}
			if logs[0]["level"] != tc.level {
				t.Errorf("expected level %s, got %v", tc.level, logs[0]["level"])
			}
		})
	}
}

func TestMiddleware_UnmatchedRoute(t *testing.T) {
	tt := setupTestTelemetry(t)

	rec := serve(newTestRouter(), "/wp-admin/"+secretOAuthToken, nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	logs := tt.stdout.withMsg(t, "request")
	if len(logs) != 1 || logs[0]["route"] != unmatchedRoute {
		t.Fatalf("expected one record with route %q, got %v", unmatchedRoute, logs)
	}
	if path := spanAttr(tt.spans.GetSpans()[0].Attributes, semconv.URLPathKey); path != unmatchedRoute {
		t.Errorf("expected url.path %q, got %q", unmatchedRoute, path)
	}
	tt.expectNoSecrets(t)
}

func TestMiddleware_RequestID(t *testing.T) {
	setupTestTelemetry(t)
	r := newTestRouter()

	t.Run("keeps a valid incoming ID", func(t *testing.T) {
		rec := serve(r, "/users/1", map[string]string{RequestIDHeader: "web-1a2b.3c_4"})
		if got := rec.Header().Get(RequestIDHeader); got != "web-1a2b.3c_4" {
			t.Errorf("expected the incoming ID, got %q", got)
		}
	})

	for name, incoming := range map[string]string{
		"missing":       "",
		"too long":      strings.Repeat("a", 65),
		"bad character": "id with spaces",
		"injection":     "x\"}{\"level\":\"ERROR",
	} {
		t.Run("generates one when "+name, func(t *testing.T) {
			rec := serve(r, "/users/1", map[string]string{RequestIDHeader: incoming})
			got := rec.Header().Get(RequestIDHeader)
			if got == "" || got == incoming || !validRequestID.MatchString(got) {
				t.Errorf("expected a fresh valid ID, got %q", got)
			}
		})
	}

	t.Run("is unique per request", func(t *testing.T) {
		a := serve(r, "/users/1", nil).Header().Get(RequestIDHeader)
		b := serve(r, "/users/1", nil).Header().Get(RequestIDHeader)
		if a == b {
			t.Errorf("expected different IDs, got %q twice", a)
		}
	})

	t.Run("is set on health checks too", func(t *testing.T) {
		if got := serve(r, "/healthz/ready", nil).Header().Get(RequestIDHeader); got == "" {
			t.Error("expected an X-Request-ID on /healthz/ready")
		}
	})
}

func TestMiddleware_SkipsHealthChecks(t *testing.T) {
	tt := setupTestTelemetry(t)

	rec := serve(newTestRouter(), "/healthz/ready", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if out := tt.stdout.String(); out != "" {
		t.Errorf("expected no log for a health check, got %s", out)
	}
	if spans := tt.spans.GetSpans(); len(spans) != 0 {
		t.Errorf("expected no span for a health check, got %d", len(spans))
	}
}

func TestMiddleware_Span(t *testing.T) {
	tt := setupTestTelemetry(t)

	serve(newTestRouter(), "/users/"+testUserID+"?oauth_token="+secretOAuthToken, nil)

	spans := tt.spans.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	s := spans[0]
	if s.Name != "GET /users/{id}" {
		t.Errorf("expected span name %q, got %q", "GET /users/{id}", s.Name)
	}
	if s.SpanKind != trace.SpanKindServer {
		t.Errorf("expected a server span, got %v", s.SpanKind)
	}
	if got := spanAttr(s.Attributes, semconv.HTTPRouteKey); got != "/users/{id}" {
		t.Errorf("expected http.route /users/{id}, got %q", got)
	}
	if got := spanAttr(s.Attributes, semconv.URLPathKey); got != "/users/{id}" {
		t.Errorf("expected url.path to be the route, got %q", got)
	}
	if text := spanText(s); strings.Contains(text, testUserID) {
		t.Errorf("expected no raw path in the span, got %s", text)
	}
	tt.expectNoSecrets(t)
}

func TestMiddleware_DoesNotLetClientsPickTheTrace(t *testing.T) {
	tt := setupTestTelemetry(t)
	const clientTrace = "4bf92f3577b34da6a3ce929d0e0e4736"

	serve(newTestRouter(), "/users/1", map[string]string{
		"traceparent": "00-" + clientTrace + "-00f067aa0ba902b7-01",
	})

	s := tt.spans.GetSpans()[0]
	if s.SpanContext.TraceID().String() == clientTrace {
		t.Error("expected a new trace, not the client's")
	}
	if s.Parent.IsValid() {
		t.Error("expected a root span")
	}
	if len(s.Links) != 1 || s.Links[0].SpanContext.TraceID().String() != clientTrace {
		t.Errorf("expected a link to the client's trace, got %v", s.Links)
	}
}

func TestMiddleware_RecoversPanics(t *testing.T) {
	tt := setupTestTelemetry(t)
	r := newTestRouter()

	rec := serve(r, "/panic", nil)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"error":"internal_error"`) {
		t.Errorf("expected the JSON error body, got %s", body)
	}

	panics := tt.stdout.withMsg(t, "panic recovered")
	if len(panics) != 1 {
		t.Fatalf("expected one panic record, got %d: %s", len(panics), tt.stdout.String())
	}
	if stack, _ := panics[0]["stack"].(string); !strings.Contains(stack, "newTestRouter") {
		t.Errorf("expected the stack to show where it panicked, got %q", stack)
	}
	if requests := tt.stdout.withMsg(t, "request"); len(requests) != 1 || requests[0]["status"] != float64(500) {
		t.Errorf("expected one request record with status 500, got %v", requests)
	}

	events := tt.sentry.Events()
	if len(events) != 1 {
		t.Fatalf("expected one Sentry event, got %d", len(events))
	}
	e := events[0]
	span := tt.spans.GetSpans()[0]
	if got := e.Contexts["trace"]["trace_id"]; got != span.SpanContext.TraceID().String() {
		t.Errorf("expected the event to link to trace %s, got %v", span.SpanContext.TraceID(), got)
	}
	if e.Tags["request_id"] != rec.Header().Get(RequestIDHeader) || e.Tags["route"] != "/panic" {
		t.Errorf("expected request_id and route tags, got %v", e.Tags)
	}
	if len(e.Exception) == 0 || e.Exception[0].Stacktrace == nil {
		t.Errorf("expected an exception with a stack trace, got %+v", e.Exception)
	}
	tt.expectNoSecrets(t)

	if rec := serve(r, "/users/1", nil); rec.Code != http.StatusOK {
		t.Errorf("expected the server to keep serving after a panic, got %d", rec.Code)
	}
}

func TestReportError(t *testing.T) {
	tt := setupTestTelemetry(t)
	r := newTestRouter()

	first := serve(r, "/fail", nil)
	second := serve(r, "/fail", nil)

	events := tt.sentry.Events()
	if len(events) != 2 {
		t.Fatalf("expected one Sentry event per failure, got %d", len(events))
	}

	spans := tt.spans.GetSpans()
	for i, rec := range []*httptest.ResponseRecorder{first, second} {
		e := events[i]
		if e.Tags["request_id"] != rec.Header().Get(RequestIDHeader) {
			t.Errorf("event %d: expected request_id %s, got %v", i, rec.Header().Get(RequestIDHeader), e.Tags)
		}
		if e.Tags["route"] != "/fail" || e.Tags["operation"] != "load profile" {
			t.Errorf("event %d: expected route and operation tags, got %v", i, e.Tags)
		}
		if got := e.Contexts["trace"]["trace_id"]; got != spans[i].SpanContext.TraceID().String() {
			t.Errorf("event %d: expected trace %s, got %v", i, spans[i].SpanContext.TraceID(), got)
		}
		if e.Request != nil || !e.User.IsEmpty() {
			t.Errorf("event %d: expected no request or user data, got %+v %+v", i, e.Request, e.User)
		}
	}

	errs := tt.stdout.withMsg(t, "load profile")
	if len(errs) != 2 || errs[0]["level"] != "ERROR" {
		t.Errorf("expected two error records, got %v", errs)
	}
	tt.expectNoSecrets(t)

	sentry.CurrentHub().CaptureMessage("probe")
	events = tt.sentry.Events()
	if _, ok := events[len(events)-1].Tags["request_id"]; ok {
		t.Error("expected request tags to stay in the request's scope")
	}
}
