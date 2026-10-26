package telemetry

import (
	"context"
	"io"
	"log/slog"

	"go.opentelemetry.io/otel/trace"
)

type Resource struct {
	Service string
	Env     string
	Version string
}

type requestIDKey struct{}

func withRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func NewLogger(w io.Writer, res Resource) *slog.Logger {
	return newLogger(newStdoutHandler(w, res), nil)
}

func newLogger(stdout, otel slog.Handler) *slog.Logger {
	out := stdout
	if otel != nil {
		out = slog.NewMultiHandler(stdout, levelHandler{next: otel, min: slog.LevelInfo})
	}
	return slog.New(redactHandler{next: requestIDHandler{next: out}})
}

func newStdoutHandler(w io.Writer, res Resource) slog.Handler {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	return traceHandler{next: h.WithAttrs([]slog.Attr{
		slog.String("service", res.Service),
		slog.String("env", res.Env),
		slog.String("version", res.Version),
	})}
}

type requestIDHandler struct {
	next slog.Handler
}

func (h requestIDHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h requestIDHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestID(ctx); id != "" {
		r = r.Clone()
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.next.Handle(ctx, r)
}

func (h requestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDHandler{next: h.next.WithAttrs(attrs)}
}

func (h requestIDHandler) WithGroup(name string) slog.Handler {
	return requestIDHandler{next: h.next.WithGroup(name)}
}

type traceHandler struct {
	next slog.Handler
}

func (h traceHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h traceHandler) Handle(ctx context.Context, r slog.Record) error {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		r = r.Clone()
		r.AddAttrs(
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}
	return h.next.Handle(ctx, r)
}

func (h traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return traceHandler{next: h.next.WithAttrs(attrs)}
}

func (h traceHandler) WithGroup(name string) slog.Handler {
	return traceHandler{next: h.next.WithGroup(name)}
}

type levelHandler struct {
	next slog.Handler
	min  slog.Level
}

func (h levelHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.min && h.next.Enabled(ctx, level)
}

func (h levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.next.Handle(ctx, r)
}

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{next: h.next.WithAttrs(attrs), min: h.min}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{next: h.next.WithGroup(name), min: h.min}
}
