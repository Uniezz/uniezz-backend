package telemetry

import (
	"cmp"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

const RequestIDHeader = "X-Request-ID"

const unmatchedRoute = "unmatched"

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func Middleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(requests(next), "http.server",
		otelhttp.WithFilter(isNotHealthCheck),
		otelhttp.WithSpanNameFormatter(spanNameBeforeRouting),
		otelhttp.WithPublicEndpointFn(ignoreClientSamplingDecision),
	)
}

func isNotHealthCheck(r *http.Request) bool {
	return !isHealthCheck(r)
}

func spanNameBeforeRouting(_ string, r *http.Request) string {
	return r.Method
}

func ignoreClientSamplingDecision(*http.Request) bool {
	return true
}

func isHealthCheck(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/healthz/")
}

func requestID(incoming string) string {
	if validRequestID.MatchString(incoming) {
		return incoming
	}
	return uuid.NewString()
}

func requests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		id := requestID(r.Header.Get(RequestIDHeader))
		w.Header().Set(RequestIDHeader, id)

		hub := sentry.CurrentHub().Clone()
		hub.Scope().SetTag("request_id", id)
		hub.Scope().SetTag("http.method", r.Method)
		ctx := sentry.SetHubOnContext(withRequestID(r.Context(), id), hub)
		r = r.WithContext(ctx)

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			if recovered := recover(); recovered != nil {
				if recovered == http.ErrAbortHandler {
					panic(recovered)
				}
				reportPanic(ctx, recovered, debug.Stack())
				if ww.Status() == 0 {
					writeInternalError(ww)
				}
			}

			if isHealthCheck(r) {
				return
			}
			logRequest(r, ww.Status(), time.Since(start))
		}()

		next.ServeHTTP(ww, r)
	})
}

func writeInternalError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal_error"})
}

func logRequest(r *http.Request, status int, duration time.Duration) {
	ctx := r.Context()
	status = cmp.Or(status, http.StatusOK)

	route := routeOf(ctx)
	if route == "" {
		route = unmatchedRoute
	}

	nameSpanByRoute(trace.SpanFromContext(ctx), r.Method, route)

	slog.Default().LogAttrs(ctx, levelFor(status), "request",
		slog.String("method", r.Method),
		slog.String("route", route),
		slog.Int("status", status),
		slog.Float64("duration_ms", float64(duration.Microseconds())/1000),
		slog.String("client_ip", clientIP(r)),
	)
}

func nameSpanByRoute(span trace.Span, method, route string) {
	span.SetName(method + " " + route)
	replaceRawPathWithRoute(span, route)
}

func replaceRawPathWithRoute(span trace.Span, route string) {
	span.SetAttributes(semconv.HTTPRoute(route), semconv.URLPath(route))
}

func levelFor(status int) slog.Level {
	switch {
	case status >= http.StatusInternalServerError:
		return slog.LevelError
	case status >= http.StatusBadRequest:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
