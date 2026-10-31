package config

import (
	"slices"
	"testing"
	"time"
)

const testOTPHashKey = "0123456789abcdef0123456789abcdef"

func loadWith(t *testing.T, env map[string]string) *Config {
	t.Helper()

	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("OTP_EMAIL_TEMPLATE_ID", "tmpl_test")
	t.Setenv("OTP_HASH_KEY", testOTPHashKey)
	t.Setenv("S3_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
	for k, v := range env {
		t.Setenv(k, v)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	return cfg
}

func TestLoad_CORSAllowedOrigins(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want []string
	}{
		{
			name: "defaults to the default web app URL",
			env:  map[string]string{"APP_WEB_URL": "", "CORS_ALLOWED_ORIGINS": ""},
			want: []string{"http://localhost:3000"},
		},
		{
			name: "defaults to APP_WEB_URL",
			env:  map[string]string{"APP_WEB_URL": "https://stg.example.com", "CORS_ALLOWED_ORIGINS": ""},
			want: []string{"https://stg.example.com"},
		},
		{
			name: "reads a comma-separated list",
			env:  map[string]string{"APP_WEB_URL": "https://stg.example.com", "CORS_ALLOWED_ORIGINS": "https://stg.example.com,http://localhost:3000"},
			want: []string{"https://stg.example.com", "http://localhost:3000"},
		},
		{
			name: "trims spaces and skips empty items",
			env:  map[string]string{"CORS_ALLOWED_ORIGINS": " https://a.example.com , ,http://localhost:3000, "},
			want: []string{"https://a.example.com", "http://localhost:3000"},
		},
		{
			// Browsers never send a trailing slash in Origin, so "https://a.example.com/" would never match.
			name: "drops trailing slashes",
			env:  map[string]string{"CORS_ALLOWED_ORIGINS": "https://a.example.com/,http://localhost:3000/"},
			want: []string{"https://a.example.com", "http://localhost:3000"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := loadWith(t, tc.env).CORSAllowedOrigins; !slices.Equal(got, tc.want) {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestLoad_S3PublicEndpoint(t *testing.T) {
	cfg := loadWith(t, map[string]string{
		"S3_PUBLIC_ENDPOINT": "http://localhost:9000",
	})

	if cfg.S3PublicEndpoint != "http://localhost:9000" {
		t.Errorf("expected public endpoint %q, got %q", "http://localhost:9000", cfg.S3PublicEndpoint)
	}
}

func TestLoad_MediaPublicURL(t *testing.T) {
	cfg := loadWith(t, map[string]string{
		"MEDIA_PUBLIC_URL": "https://media.uniezz.pl",
	})

	if cfg.MediaPublicURL != "https://media.uniezz.pl" {
		t.Errorf("expected public media URL %q, got %q", "https://media.uniezz.pl", cfg.MediaPublicURL)
	}
}

func TestLoad_APPWebURLStaysSingle(t *testing.T) {
	// APP_WEB_URL is also the post-login redirect target, so a CORS list must not leak into it.
	cfg := loadWith(t, map[string]string{
		"APP_WEB_URL":          "https://stg.example.com",
		"CORS_ALLOWED_ORIGINS": "https://stg.example.com,http://localhost:3000",
	})

	if cfg.AppWebURL != "https://stg.example.com" {
		t.Errorf("expected APP_WEB_URL to stay %q, got %q", "https://stg.example.com", cfg.AppWebURL)
	}
}

func TestLoad_RequiresOTPEmailTemplateID(t *testing.T) {
	// A missing template ID would only surface when the first OTP email fails, so startup must refuse it.
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("OTP_EMAIL_TEMPLATE_ID", "")
	t.Setenv("OTP_HASH_KEY", testOTPHashKey)

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a missing OTP_EMAIL_TEMPLATE_ID, got nil")
	}
}

func TestLoad_RequiresLongEnoughOTPHashKey(t *testing.T) {
	for _, key := range []string{"", "too-short"} {
		t.Setenv("DATABASE_URL", "postgres://test")
		t.Setenv("RESEND_API_KEY", "re_test")
		t.Setenv("OTP_EMAIL_TEMPLATE_ID", "tmpl_test")
		t.Setenv("OTP_HASH_KEY", key)

		if _, err := Load(); err == nil {
			t.Errorf("expected an error for OTP_HASH_KEY %q, got nil", key)
		}
	}
}

func TestLoad_OTPTestEmails(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  string
		want []string
	}{
		{name: "unset", env: "", want: nil},
		{name: "normalizes and skips empty items", env: " Nikita@Gmail.com , ,kyrylo@gmail.com, ", want: []string{"nikita@gmail.com", "kyrylo@gmail.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := loadWith(t, map[string]string{"OTP_TEST_EMAILS": tc.env}).OTPTestEmails
			if !slices.Equal(got, tc.want) {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestLoad_OTPTestEmailsRejectedInProduction(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("RESEND_API_KEY", "re_test")
	t.Setenv("OTP_EMAIL_TEMPLATE_ID", "tmpl_test")
	t.Setenv("OTP_HASH_KEY", testOTPHashKey)
	t.Setenv("ENV", "production")
	t.Setenv("OTP_TEST_EMAILS", "nikita@gmail.com")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for OTP_TEST_EMAILS in production, got nil")
	}
}

func TestLoad_OTPTestEmailsAllowedOutsideProduction(t *testing.T) {
	for _, env := range []string{"development", "staging"} {
		cfg := loadWith(t, map[string]string{"ENV": env, "OTP_TEST_EMAILS": "nikita@gmail.com"})
		if len(cfg.OTPTestEmails) != 1 {
			t.Errorf("%s: expected the test email to load, got %q", env, cfg.OTPTestEmails)
		}
	}
}

func TestLoad_Telemetry(t *testing.T) {
	t.Run("off by default", func(t *testing.T) {
		cfg := loadWith(t, map[string]string{"OTEL_SERVICE_NAME": "", "SENTRY_DSN": "", "SENTRY_ENVIRONMENT": "", "SENTRY_RELEASE": ""})

		if cfg.ServiceName != DefaultServiceName {
			t.Errorf("expected service name %q, got %q", DefaultServiceName, cfg.ServiceName)
		}
		if cfg.SentryDSN != "" || cfg.SentryEnvironment != "" || cfg.SentryRelease != "" {
			t.Errorf("expected no Sentry settings, got %q %q %q", cfg.SentryDSN, cfg.SentryEnvironment, cfg.SentryRelease)
		}
	})

	t.Run("reads the variables", func(t *testing.T) {
		cfg := loadWith(t, map[string]string{
			"OTEL_SERVICE_NAME":  "uniezz-worker",
			"SENTRY_DSN":         "https://public@o1.ingest.sentry.io/1",
			"SENTRY_ENVIRONMENT": "staging",
			"SENTRY_RELEASE":     "abc1234",
		})

		if cfg.ServiceName != "uniezz-worker" || cfg.SentryDSN != "https://public@o1.ingest.sentry.io/1" ||
			cfg.SentryEnvironment != "staging" || cfg.SentryRelease != "abc1234" {
			t.Errorf("unexpected telemetry config: %+v", cfg)
		}
	})
}

func TestLoad_RequiresS3Credentials(t *testing.T) {
	for _, tc := range []struct{ key, secret string }{{"", "secret"}, {"key", ""}} {
		t.Setenv("DATABASE_URL", "postgres://test")
		t.Setenv("RESEND_API_KEY", "re_test")
		t.Setenv("OTP_EMAIL_TEMPLATE_ID", "tmpl_test")
		t.Setenv("OTP_HASH_KEY", testOTPHashKey)
		t.Setenv("S3_ACCESS_KEY_ID", tc.key)
		t.Setenv("S3_SECRET_ACCESS_KEY", tc.secret)

		if _, err := Load(); err == nil {
			t.Errorf("expected an error for S3 key %q and secret %q, got nil", tc.key, tc.secret)
		}
	}
}

func TestLoad_JobPollInterval(t *testing.T) {
	if got := loadWith(t, nil).JobPollInterval; got != 30*time.Second {
		t.Errorf("expected the 30s default, got %v", got)
	}
	if got := loadWith(t, map[string]string{"JOB_POLL_INTERVAL": "5m"}).JobPollInterval; got != 5*time.Minute {
		t.Errorf("expected 5m, got %v", got)
	}

	for _, bad := range []string{"soon", "500ms"} {
		t.Setenv("JOB_POLL_INTERVAL", bad)
		t.Setenv("DATABASE_URL", "postgres://test")
		t.Setenv("RESEND_API_KEY", "re_test")
		t.Setenv("OTP_EMAIL_TEMPLATE_ID", "tmpl_test")
		t.Setenv("OTP_HASH_KEY", testOTPHashKey)
		t.Setenv("S3_ACCESS_KEY_ID", "test-access-key")
		t.Setenv("S3_SECRET_ACCESS_KEY", "test-secret-key")
		if _, err := Load(); err == nil {
			t.Errorf("expected an error for JOB_POLL_INTERVAL=%q", bad)
		}
	}
}
