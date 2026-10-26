package telemetry

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func clearTelemetryEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OTEL_SDK_DISABLED",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_TRACES_HEADERS",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT",
		"OTEL_EXPORTER_OTLP_LOGS_HEADERS",
		"OTEL_TRACES_SAMPLER",
		"OTEL_TRACES_SAMPLER_ARG",
	} {
		t.Setenv(key, "")
	}
}

// restoreGlobals undoes what Setup installs globally.
func restoreGlobals(t *testing.T) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		sentry.CurrentHub().BindClient(nil)
	})
}

func TestSetup_OffWithoutConfig(t *testing.T) {
	clearTelemetryEnv(t)
	restoreGlobals(t)
	sentry.CurrentHub().BindClient(nil)
	prevTP := otel.GetTracerProvider()

	stdout := &syncBuffer{}
	tel, err := Setup(context.Background(), Options{Resource: testResource, Stdout: stdout})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if otel.GetTracerProvider() != prevTP {
		t.Error("expected no tracer provider without an OTLP endpoint")
	}
	if c := sentry.CurrentHub().Client(); c != nil && c.IsEnabled() {
		t.Error("expected Sentry to stay off without a DSN")
	}
	if len(tel.shutdowns) != 0 {
		t.Errorf("expected nothing to shut down, got %d exporters", len(tel.shutdowns))
	}

	tel.Logger.Info("hello")
	recs := stdout.withMsg(t, "hello")
	if len(recs) != 1 || recs[0]["service"] != testResource.Service || recs[0]["version"] != testResource.Version {
		t.Errorf("expected a JSON record with the resource fields, got %s", stdout.String())
	}

	if err := tel.Shutdown(context.Background()); err != nil {
		t.Errorf("expected a clean shutdown, got: %v", err)
	}
}

func TestSetup_OTelSDKDisabled(t *testing.T) {
	clearTelemetryEnv(t)
	restoreGlobals(t)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "http://127.0.0.1:1/v1/traces")
	t.Setenv("OTEL_SDK_DISABLED", "true")

	tel, err := Setup(context.Background(), Options{Resource: testResource, Stdout: io.Discard})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(tel.shutdowns) != 0 {
		t.Errorf("expected no exporters with OTEL_SDK_DISABLED=true, got %d", len(tel.shutdowns))
	}
}

type otlpRequest struct {
	path, auth string
	body       string
}

// fakeOTLP records what the exporters send, as the Sentry OTLP endpoint would receive it.
type fakeOTLP struct {
	mu       sync.Mutex
	requests []otlpRequest
}

func (f *fakeOTLP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var body io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body = gz
	}
	b, _ := io.ReadAll(body)

	f.mu.Lock()
	f.requests = append(f.requests, otlpRequest{path: r.URL.Path, auth: r.Header.Get("X-Sentry-Auth"), body: string(b)})
	f.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

func (f *fakeOTLP) to(path string) []otlpRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []otlpRequest
	for _, r := range f.requests {
		if r.path == path {
			out = append(out, r)
		}
	}
	return out
}

func TestSetup_ExportsToTheConfiguredEndpoints(t *testing.T) {
	clearTelemetryEnv(t)
	restoreGlobals(t)

	otlp := &fakeOTLP{}
	srv := httptest.NewServer(otlp)
	t.Cleanup(srv.Close)

	// The complete URLs from Sentry's OpenTelemetry tab, used as they are.
	const tracesPath = "/api/1/integration/otlp/v1/traces"
	const logsPath = "/api/1/integration/otlp/v1/logs"
	const auth = "sentry sentry_key=public123"
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", srv.URL+tracesPath)
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "x-sentry-auth="+auth)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", srv.URL+logsPath)
	t.Setenv("OTEL_EXPORTER_OTLP_LOGS_HEADERS", "x-sentry-auth="+auth)
	t.Setenv("OTEL_TRACES_SAMPLER", "always_on")

	stdout := &syncBuffer{}
	tel, err := Setup(context.Background(), Options{Resource: testResource, Stdout: stdout})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if _, ok := otel.GetTracerProvider().(*sdktrace.TracerProvider); !ok {
		t.Fatalf("expected an SDK tracer provider, got %T", otel.GetTracerProvider())
	}

	ctx, span := otel.Tracer("test").Start(context.Background(), "work")
	tel.Logger.InfoContext(ctx, "did work", "session_token", secretSessionToken, "error", "oauth_verifier="+secretVerifier)
	span.End()

	if err := tel.Shutdown(context.Background()); err != nil {
		t.Fatalf("expected a clean shutdown, got: %v", err)
	}

	for name, path := range map[string]string{"traces": tracesPath, "logs": logsPath} {
		reqs := otlp.to(path)
		if len(reqs) == 0 {
			t.Errorf("expected %s at %s, got requests to %v", name, path, otlp.requests)
			continue
		}
		for _, r := range reqs {
			if r.auth != auth {
				t.Errorf("%s: expected X-Sentry-Auth %q, got %q", name, auth, r.auth)
			}
			for _, secret := range allSecrets {
				if strings.Contains(r.body, secret) {
					t.Errorf("%s: payload contains the secret %q", name, secret)
				}
			}
		}
	}
	if logs := otlp.to(logsPath); len(logs) > 0 {
		body := logs[0].body
		for _, want := range []string{"did work", testResource.Service, testResource.Version, testResource.Env} {
			if !strings.Contains(body, want) {
				t.Errorf("expected the exported log to contain %q", want)
			}
		}
	}
}
