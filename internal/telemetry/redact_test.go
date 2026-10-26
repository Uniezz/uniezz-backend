package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestScrub(t *testing.T) {
	for _, tc := range []struct {
		name, in, secret string
	}{
		{
			name:   "USOS callback query",
			in:     "GET /auth/usos/callback?oauth_token=" + secretOAuthToken + "&oauth_verifier=" + secretVerifier,
			secret: secretVerifier,
		},
		{
			name:   "request token in a query",
			in:     "authorize?oauth_token=" + secretOAuthToken,
			secret: secretOAuthToken,
		},
		{
			name:   "login code in a redirect",
			in:     "redirect to https://uniezz.pl/auth/done?code=" + secretOTP,
			secret: secretOTP,
		},
		{
			name:   "bearer token",
			in:     "Authorization: Bearer " + secretSessionToken,
			secret: secretSessionToken,
		},
		{
			name:   "OAuth 1 header",
			in:     `OAuth oauth_consumer_key="key", oauth_token="` + secretOAuthToken + `", oauth_signature="sig"`,
			secret: secretOAuthToken,
		},
		{
			name:   "JSON body quoted in an error",
			in:     `unexpected response {"token":"` + secretSessionToken + `","user":"x"}`,
			secret: secretSessionToken,
		},
		{
			name:   "OTP code in a JSON body",
			in:     `{"email":"a@b.pl","code": "` + secretOTP + `"}`,
			secret: secretOTP,
		},
		{
			name:   "password in a connection string",
			in:     "connect postgres://app:" + secretDBPassword + "@db.neon.tech/app failed",
			secret: secretDBPassword,
		},
		{
			name:   "consumer secret in a form",
			in:     "body: consumer_key=k&secret=" + secretConsumer,
			secret: secretConsumer,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Scrub(tc.in)
			if strings.Contains(got, tc.secret) {
				t.Errorf("Scrub(%q) = %q, still holds the secret", tc.in, got)
			}
			if !strings.Contains(got, redacted) {
				t.Errorf("Scrub(%q) = %q, expected a %s marker", tc.in, got, redacted)
			}
		})
	}
}

func TestScrub_KeepsOrdinaryText(t *testing.T) {
	for _, in := range []string{
		"pq: connection to 10.0.0.5 refused",
		"finish USOS login for \"umcs\": usos api returned status: 503",
		"student_status=1",
		"GET /auth/usos/callback",
	} {
		if got := Scrub(in); got != in {
			t.Errorf("Scrub(%q) = %q, expected it unchanged", in, got)
		}
	}
}

func TestIsSensitiveKey(t *testing.T) {
	for key, want := range map[string]bool{
		"authorization":      true,
		"Authorization":      true,
		"cookie":             true,
		"Set-Cookie":         true,
		"session_token":      true,
		"token":              true,
		"oauth_verifier":     true,
		"consumer_secret":    true,
		"otp":                true,
		"otp_code":           true,
		"login_code":         true,
		"code":               true,
		"password":           true,
		"sentry_dsn":         true,
		"error_code":         false,
		"status":             false,
		"route":              false,
		"request_id":         false,
		"tokens_issued_hint": false,
	} {
		if got := isSensitiveKey(key); got != want {
			t.Errorf("isSensitiveKey(%q) = %v, want %v", key, got, want)
		}
	}
}

type loginAttempt struct {
	Email string
	Code  string
}

func TestLogger_RedactsSecretsInEveryOutput(t *testing.T) {
	tt := setupTestTelemetry(t)

	slog.Default().With("session_token", secretSessionToken).InfoContext(context.Background(), "login",
		"authorization", "Bearer "+secretSessionToken,
		"otp_code", secretOTP,
		"code", secretOTP,
		"error_code", "invalid_code",
		slog.Group("usos", "oauth_verifier", secretVerifier, "consumer_secret", secretConsumer),
		"error", fmt.Errorf("access token: %w", errors.New("oauth_token="+secretOAuthToken)),
		"attempt", loginAttempt{Email: "a@umcs.pl", Code: "code=" + secretOTP},
		"url", "/auth/usos/callback?oauth_verifier="+secretVerifier,
	)

	tt.expectNoSecrets(t)

	recs := tt.stdout.withMsg(t, "login")
	if len(recs) != 1 {
		t.Fatalf("expected one stdout record, got %d: %s", len(recs), tt.stdout.String())
	}
	if recs[0]["error_code"] != "invalid_code" {
		t.Errorf("expected error_code to be kept, got %v", recs[0]["error_code"])
	}
	if recs[0]["session_token"] != redacted || recs[0]["authorization"] != redacted {
		t.Errorf("expected sensitive keys to be redacted, got %v", recs[0])
	}
	if len(tt.logs.all()) != 1 {
		t.Errorf("expected one exported record, got %d", len(tt.logs.all()))
	}
}
