package database_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNewPool_Integration_QueriesAreChildSpansWithoutBindValues(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping integration test because DATABASE_URL is not set")
	}

	spans := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	defer pool.Close()

	const secret = "sess_bind_value_must_not_leak"
	reqCtx, parent := otel.Tracer("test").Start(ctx, "GET /test")
	var got string
	if err := pool.QueryRow(reqCtx, "SELECT $1::text", secret).Scan(&got); err != nil {
		t.Fatalf("query: %v", err)
	}
	parent.End()

	var query *tracetest.SpanStub
	for _, s := range spans.GetSpans() {
		for _, kv := range s.Attributes {
			if strings.Contains(kv.Value.String(), secret) || strings.Contains(s.Name, secret) {
				t.Errorf("span %q holds the bind value in %s", s.Name, kv.Key)
			}
		}
		if s.Parent.SpanID() == parent.SpanContext().SpanID() {
			query = &s
		}
	}
	if query == nil {
		t.Fatalf("expected the query as a child span of the request span, got %d spans", len(spans.GetSpans()))
	}

	var text string
	for _, kv := range query.Attributes {
		if kv.Key == "db.query.text" {
			text = kv.Value.String()
		}
	}
	if text != "SELECT $1::text" {
		t.Errorf("expected the parameterized statement in db.query.text, got %q", text)
	}
}
