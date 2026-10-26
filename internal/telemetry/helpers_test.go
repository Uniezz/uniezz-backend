package telemetry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Representative secrets: none of them may show up in any output.
const (
	secretSessionToken = "sess_9f8e7d6c5b4a39281706f5e4d3c2b1a0"
	secretOAuthToken   = "rtok_Zx81QwErTy"
	secretVerifier     = "ver_31337abcdef"
	secretOTP          = "482913"
	secretConsumer     = "cs_UsosConsumerSecret_77"
	secretDBPassword   = "pg_p4ssw0rd_42"
)

var allSecrets = []string{secretSessionToken, secretOAuthToken, secretVerifier, secretOTP, secretConsumer, secretDBPassword}

var testResource = Resource{Service: "uniezz-api-test", Env: "test", Version: "abc1234"}

// syncBuffer is a bytes.Buffer safe for the exporters' goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// records parses the JSON lines written to stdout.
func (b *syncBuffer) records(t *testing.T) []map[string]any {
	t.Helper()

	var out []map[string]any
	sc := bufio.NewScanner(strings.NewReader(b.String()))
	for sc.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("stdout line is not JSON: %v\n%s", err, sc.Text())
		}
		out = append(out, rec)
	}
	return out
}

func (b *syncBuffer) withMsg(t *testing.T, msg string) []map[string]any {
	t.Helper()

	var out []map[string]any
	for _, rec := range b.records(t) {
		if rec["msg"] == msg {
			out = append(out, rec)
		}
	}
	return out
}

// logRecorder is an OTel log exporter that keeps what it is given.
type logRecorder struct {
	mu      sync.Mutex
	records []sdklog.Record
}

func (r *logRecorder) Export(_ context.Context, records []sdklog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, rec := range records {
		r.records = append(r.records, rec.Clone())
	}
	return nil
}

func (r *logRecorder) Shutdown(context.Context) error   { return nil }
func (r *logRecorder) ForceFlush(context.Context) error { return nil }

func (r *logRecorder) all() []sdklog.Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sdklog.Record(nil), r.records...)
}

// text renders an exported record as one string, to search it for secrets.
func recordText(rec sdklog.Record) string {
	var sb strings.Builder
	sb.WriteString(rec.Body().String())
	rec.WalkAttributes(func(kv attribute.KeyValue) bool {
		sb.WriteString(" " + string(kv.Key) + "=" + kv.Value.String())
		return true
	})
	return sb.String()
}

func spanText(s tracetest.SpanStub) string {
	var sb strings.Builder
	sb.WriteString(s.Name)
	for _, kv := range s.Attributes {
		sb.WriteString(" " + string(kv.Key) + "=" + kv.Value.String())
	}
	for _, ev := range s.Events {
		sb.WriteString(" " + ev.Name)
		for _, kv := range ev.Attributes {
			sb.WriteString(" " + string(kv.Key) + "=" + kv.Value.String())
		}
	}
	sb.WriteString(" " + s.Status.Description)
	return sb.String()
}

func eventText(t *testing.T, e *sentry.Event) string {
	t.Helper()

	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal sentry event: %v", err)
	}
	return string(b)
}

// testTelemetry wires every output to in-memory recorders: stdout, OTel spans,
// OTel logs and Sentry events.
type testTelemetry struct {
	stdout *syncBuffer
	spans  *tracetest.InMemoryExporter
	logs   *logRecorder
	sentry *sentry.MockTransport
}

func setupTestTelemetry(t *testing.T) *testTelemetry {
	t.Helper()

	tt := &testTelemetry{
		stdout: &syncBuffer{},
		spans:  tracetest.NewInMemoryExporter(),
		logs:   &logRecorder{},
		sentry: &sentry.MockTransport{},
	}

	installPropagator()
	prevTP := otel.GetTracerProvider()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(tt.spans))
	otel.SetTracerProvider(tp)

	lp := sdklog.NewLoggerProvider(sdklog.WithProcessor(sdklog.NewSimpleProcessor(tt.logs)))
	prevLogger := slog.Default()
	slog.SetDefault(newLogger(
		newStdoutHandler(tt.stdout, testResource),
		otelslog.NewHandler(instrumentationName, otelslog.WithLoggerProvider(lp)),
	))

	opts := sentryOptions(Options{Resource: testResource, SentryDSN: "https://public@o1.ingest.sentry.io/1"})
	opts.Transport = tt.sentry
	if err := sentry.Init(opts); err != nil {
		t.Fatalf("init sentry: %v", err)
	}

	t.Cleanup(func() {
		sentry.CurrentHub().BindClient(nil)
		slog.SetDefault(prevLogger)
		otel.SetTracerProvider(prevTP)
		_ = tp.Shutdown(context.Background())
	})
	return tt
}

// expectNoSecrets fails when any output holds one of the representative secrets.
func (tt *testTelemetry) expectNoSecrets(t *testing.T) {
	t.Helper()

	outputs := map[string]string{"stdout": tt.stdout.String()}
	for i, rec := range tt.logs.all() {
		outputs[fmt.Sprintf("exported log %d", i)] = recordText(rec)
	}
	for _, s := range tt.spans.GetSpans() {
		outputs["span "+s.Name] = spanText(s)
	}
	for _, e := range tt.sentry.Events() {
		outputs["sentry event "+string(e.EventID)] = eventText(t, e)
	}

	for name, text := range outputs {
		for _, secret := range allSecrets {
			if strings.Contains(text, secret) {
				t.Errorf("%s contains the secret %q:\n%s", name, secret, text)
			}
		}
	}
}
