package telemetry

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"

	"github.com/getsentry/sentry-go"
	sentryotel "github.com/getsentry/sentry-go/otel"
	"github.com/go-chi/chi/v5"
)

func sentryOptions(opts Options) sentry.ClientOptions {
	off := &sentry.KeyValueCollectionBehavior{Mode: sentry.CollectionOff}
	return sentry.ClientOptions{
		Dsn:         opts.SentryDSN,
		Environment: cmp.Or(opts.SentryEnvironment, opts.Env),
		Release:     cmp.Or(opts.SentryRelease, opts.Version),
		DataCollection: &sentry.DataCollection{
			UserInfo:    sentry.Set(false),
			Cookies:     off,
			HTTPHeaders: &sentry.HeaderCollectionConfig{Request: off, Response: off},
			HTTPBodies:  []sentry.BodyType{},
			QueryParams: off,
		},
		MaxBreadcrumbs: -1,
		BeforeSend:     scrubEvent,
		Integrations: func(integrations []sentry.Integration) []sentry.Integration {
			return append(integrations, sentryotel.NewOtelIntegration())
		},
	}
}

func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	event.Request = nil
	event.User = sentry.User{}
	event.Breadcrumbs = nil
	event.Message = Scrub(event.Message)
	for i := range event.Exception {
		event.Exception[i].Value = Scrub(event.Exception[i].Value)
	}
	for k, v := range event.Tags {
		if isSensitiveKey(k) {
			event.Tags[k] = redacted
		} else {
			event.Tags[k] = Scrub(v)
		}
	}
	return event
}

func hubFor(ctx context.Context) *sentry.Hub {
	if hub := sentry.GetHubFromContext(ctx); hub != nil {
		return hub
	}
	return sentry.CurrentHub()
}

func ReportError(ctx context.Context, msg string, err error, attrs ...slog.Attr) {
	attrs = append(attrs, slog.Any("error", err))
	slog.Default().LogAttrs(ctx, slog.LevelError, msg, attrs...)

	hub := hubFor(ctx)
	client := hub.Client()
	if client == nil || !client.IsEnabled() {
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("operation", msg)
		if route := routeOf(ctx); route != "" {
			scope.SetTag("route", route)
		}
		client.CaptureException(err, &sentry.EventHint{Context: ctx, OriginalException: err}, scope)
	})
}

func reportPanic(ctx context.Context, recovered any, stack []byte) {
	slog.Default().LogAttrs(ctx, slog.LevelError, "panic recovered",
		slog.String("panic", fmt.Sprint(recovered)),
		slog.String("stack", string(stack)),
	)

	hub := hubFor(ctx)
	client := hub.Client()
	if client == nil || !client.IsEnabled() {
		return
	}

	err, ok := recovered.(error)
	if !ok {
		err = fmt.Errorf("panic: %v", recovered)
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
		if route := routeOf(ctx); route != "" {
			scope.SetTag("route", route)
		}
		scope.AddEventProcessor(func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			if n := len(event.Exception); n > 0 {
				event.Exception[n-1].Mechanism = &sentry.Mechanism{Type: "panic"}
				event.Exception[n-1].Mechanism.SetUnhandled()
			}
			return event
		})
		client.CaptureException(err, &sentry.EventHint{Context: ctx, RecoveredException: recovered}, scope)
	})
}

func routeOf(ctx context.Context) string {
	if rc := chi.RouteContext(ctx); rc != nil {
		return rc.RoutePattern()
	}
	return ""
}
