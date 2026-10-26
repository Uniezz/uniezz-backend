package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

const redacted = "[REDACTED]"

var sensitiveKeys = []string{
	"authorization", "cookie", "set_cookie", "password", "secret", "token",
	"verifier", "signature", "otp", "otp_code", "login_code", "api_key", "dsn",
}

var exactSensitiveKeys = []string{"code"}

func isSensitiveKey(key string) bool {
	key = strings.ReplaceAll(strings.ToLower(key), "-", "_")
	for _, term := range sensitiveKeys {
		if key == term || strings.HasSuffix(key, "_"+term) {
			return true
		}
	}
	for _, term := range exactSensitiveKeys {
		if key == term {
			return true
		}
	}
	return false
}

var scrubRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	{
		regexp.MustCompile(`(?i)\b(oauth_token|oauth_token_secret|oauth_verifier|oauth_signature|oauth_consumer_key|access_token|refresh_token|token|code|otp|secret|password|verifier|api_key)=[^&\s"',;]+`),
		"${1}=" + redacted,
	},
	{
		regexp.MustCompile(`(?i)\b(oauth_[a-z_]+)="[^"]*"`),
		`${1}="` + redacted + `"`,
	},
	{
		regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]+`),
		"${1} " + redacted,
	},
	{
		regexp.MustCompile(`(?i)"(token|session_token|code|otp|secret|password|access_token|refresh_token)"\s*:\s*"[^"]*"`),
		`"${1}":"` + redacted + `"`,
	},
	{
		regexp.MustCompile(`://[^/\s:@]+(:[^/\s@]*)?@`),
		"://" + redacted + "@",
	},
}

func Scrub(s string) string {
	for _, rule := range scrubRules {
		s = rule.re.ReplaceAllString(s, rule.repl)
	}
	return s
}

func redactAttr(a slog.Attr) slog.Attr {
	a.Value = a.Value.Resolve()
	if isSensitiveKey(a.Key) {
		return slog.String(a.Key, redacted)
	}

	switch a.Value.Kind() {
	case slog.KindString:
		return slog.String(a.Key, Scrub(a.Value.String()))
	case slog.KindGroup:
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(redactAttrs(a.Value.Group())...)}
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			return slog.String(a.Key, Scrub(err.Error()))
		}
		return slog.String(a.Key, Scrub(fmt.Sprint(a.Value.Any())))
	default:
		return a
	}
}

func redactAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = redactAttr(a)
	}
	return out
}

type redactHandler struct {
	next slog.Handler
}

func (h redactHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h redactHandler) Handle(ctx context.Context, r slog.Record) error {
	out := slog.NewRecord(r.Time, r.Level, Scrub(r.Message), r.PC)
	r.Attrs(func(a slog.Attr) bool {
		out.AddAttrs(redactAttr(a))
		return true
	})
	return h.next.Handle(ctx, out)
}

func (h redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactHandler{next: h.next.WithAttrs(redactAttrs(attrs))}
}

func (h redactHandler) WithGroup(name string) slog.Handler {
	return redactHandler{next: h.next.WithGroup(name)}
}
