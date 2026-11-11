package telemetry

import (
	"context"
	"log/slog"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/getsentry/sentry-go/attribute"
)

func RecordCount(ctx context.Context, name string, value int64, attrs ...slog.Attr) {
	sentry.NewMeter(ctx).Count(name, value, sentry.WithAttributes(metricAttributes(attrs)...))
}

func RecordGauge(ctx context.Context, name string, value float64, attrs ...slog.Attr) {
	sentry.NewMeter(ctx).Gauge(name, value, sentry.WithAttributes(metricAttributes(attrs)...))
}

func RecordDuration(ctx context.Context, name string, d time.Duration, attrs ...slog.Attr) {
	sentry.NewMeter(ctx).Distribution(name, float64(d.Milliseconds()),
		sentry.WithUnit(sentry.UnitMillisecond), sentry.WithAttributes(metricAttributes(attrs)...))
}

func metricAttributes(attrs []slog.Attr) []attribute.Builder {
	out := make([]attribute.Builder, 0, len(attrs))
	for _, a := range attrs {
		v := a.Value.Resolve()
		switch v.Kind() {
		case slog.KindInt64:
			out = append(out, attribute.Int64(a.Key, v.Int64()))
		case slog.KindFloat64:
			out = append(out, attribute.Float64(a.Key, v.Float64()))
		case slog.KindBool:
			out = append(out, attribute.Bool(a.Key, v.Bool()))
		default:
			out = append(out, attribute.String(a.Key, v.String()))
		}
	}
	return out
}
