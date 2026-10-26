package telemetry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/getsentry/sentry-go"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

const instrumentationName = "github.com/Uniezz/uniezz-backend"

type Options struct {
	Resource

	SentryDSN         string
	SentryEnvironment string
	SentryRelease     string

	Stdout io.Writer
}

type Telemetry struct {
	Logger *slog.Logger

	shutdowns []func(context.Context) error
}

func Setup(ctx context.Context, opts Options) (*Telemetry, error) {
	t := &Telemetry{}
	stdout := newStdoutHandler(opts.Stdout, opts.Resource)

	exporterErrorLog := slog.New(redactHandler{next: stdout})
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		exporterErrorLog.Warn("opentelemetry", "error", err)
	}))
	installPropagator()

	res, err := newResource(ctx, opts.Resource)
	if err != nil {
		return nil, err
	}

	if exportEnabled("TRACES") {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		tp := sdktrace.NewTracerProvider(sdktrace.WithBatcher(exporter), sdktrace.WithResource(res))
		otel.SetTracerProvider(tp)
		t.shutdowns = append(t.shutdowns, tp.Shutdown)
	}

	var otelLogs slog.Handler
	if exportEnabled("LOGS") {
		exporter, err := otlploghttp.New(ctx)
		if err != nil {
			_ = t.Shutdown(ctx)
			return nil, fmt.Errorf("create OTLP log exporter: %w", err)
		}
		lp := sdklog.NewLoggerProvider(
			sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
			sdklog.WithResource(res),
		)
		otelLogs = otelslog.NewHandler(instrumentationName, otelslog.WithLoggerProvider(lp))
		t.shutdowns = append(t.shutdowns, lp.Shutdown)
	}

	if opts.SentryDSN != "" {
		if err := sentry.Init(sentryOptions(opts)); err != nil {
			_ = t.Shutdown(ctx)
			return nil, fmt.Errorf("initialize sentry: %w", err)
		}
		t.shutdowns = append(t.shutdowns, func(ctx context.Context) error {
			if !sentry.FlushWithContext(ctx) {
				return errors.New("sentry: flush timed out")
			}
			return nil
		})
	}

	t.Logger = newLogger(stdout, otelLogs)
	return t, nil
}

func (t *Telemetry) Shutdown(ctx context.Context) error {
	var errs []error
	for i := len(t.shutdowns) - 1; i >= 0; i-- {
		errs = append(errs, t.shutdowns[i](ctx))
	}
	t.shutdowns = nil
	return errors.Join(errs...)
}

func newResource(ctx context.Context, r Resource) (*resource.Resource, error) {
	res, err := resource.New(ctx,
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			semconv.ServiceName(r.Service),
			semconv.ServiceVersion(r.Version),
			semconv.DeploymentEnvironmentName(r.Env),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build OTel resource: %w", err)
	}
	return res, nil
}

func installPropagator() {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
}

func exportEnabled(signal string) bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("OTEL_SDK_DISABLED")), "true") {
		return false
	}
	return os.Getenv("OTEL_EXPORTER_OTLP_"+signal+"_ENDPOINT") != "" ||
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
}
