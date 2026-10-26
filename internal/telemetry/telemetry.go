// Package telemetry sets up logging, tracing and error reporting: JSON logs on
// stdout, OpenTelemetry traces and logs over OTLP, and Sentry error events.
//
// The OTLP exporters are configured by the standard OTEL_EXPORTER_OTLP_*
// variables and stay off when no endpoint is set; Sentry stays off without a
// DSN. Local development and tests need neither.
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
	SentryEnvironment string // defaults to Env
	SentryRelease     string // defaults to Version, the build SHA

	Stdout io.Writer
}

type Telemetry struct {
	Logger *slog.Logger

	shutdowns []func(context.Context) error
}

// Setup starts the exporters that are configured and returns the logger to
// use. Call Shutdown before exiting to flush what is buffered.
func Setup(ctx context.Context, opts Options) (*Telemetry, error) {
	t := &Telemetry{}
	stdout := newStdoutHandler(opts.Stdout, opts.Resource)

	// Exporter failures go to stdout only: sending them through the failing exporter would loop.
	stdoutLogger := slog.New(redactHandler{next: stdout})
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		stdoutLogger.Warn("opentelemetry", "error", err)
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
		// The sampler comes from OTEL_TRACES_SAMPLER and OTEL_TRACES_SAMPLER_ARG.
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
