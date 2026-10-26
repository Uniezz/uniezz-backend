package config

import (
	"slices"
	"testing"
)

func loadWith(t *testing.T, env map[string]string) *Config {
	t.Helper()

	t.Setenv("DATABASE_URL", "postgres://test")
	t.Setenv("RESEND_API_KEY", "re_test")
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
