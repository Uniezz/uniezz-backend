package media

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/config"
)

func newTestStorage(t *testing.T) *objectStorage {
	t.Helper()

	storage, err := newObjectStorage(context.Background(), &config.Config{
		S3Region:          "us-east-1",
		S3Bucket:          "uniezz-media",
		S3AccessKeyID:     "test-key",
		S3SecretAccessKey: "test-secret",
		S3Endpoint:        "http://minio:9000",
		S3PublicEndpoint:  "http://localhost:9000",
		S3ForcePathStyle:  true,
		MediaPublicURL:    "https://media.uniezz.pl/",
	})
	if err != nil {
		t.Fatalf("newObjectStorage() error = %v", err)
	}
	return storage
}

func TestPublicEndpoint(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.Config
		want string
	}{
		{"uses public endpoint", &config.Config{S3Endpoint: "http://minio:9000", S3PublicEndpoint: "http://localhost:9000"}, "http://localhost:9000"},
		{"falls back to S3 endpoint", &config.Config{S3Endpoint: "https://s3.example.com"}, "https://s3.example.com"},
		{"empty means default AWS endpoint", &config.Config{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicEndpoint(tt.cfg); got != tt.want {
				t.Errorf("publicEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPresignPut(t *testing.T) {
	got, err := newTestStorage(t).presignPut(context.Background(), "avatars/1.jpg", "image/jpeg", 1024)
	if err != nil {
		t.Fatalf("presignPut() error = %v", err)
	}

	parsed := mustParseURL(t, got)
	if parsed.Host != "localhost:9000" || parsed.Path != "/uniezz-media/avatars/1.jpg" {
		t.Errorf("URL = %s, want localhost:9000/uniezz-media/avatars/1.jpg", got)
	}
	if parsed.Query().Get("X-Amz-Expires") != "600" {
		t.Errorf("expiry = %q, want 600 seconds", parsed.Query().Get("X-Amz-Expires"))
	}
	signed := parsed.Query().Get("X-Amz-SignedHeaders")
	for _, header := range []string{"content-type", "content-length", "if-none-match"} {
		if !strings.Contains(signed, header) {
			t.Errorf("signed headers = %q, want %s", signed, header)
		}
	}
}

func TestPresignGet(t *testing.T) {
	got, err := newTestStorage(t).presignGet(context.Background(), "chat/1.jpg")
	if err != nil {
		t.Fatalf("presignGet() error = %v", err)
	}

	parsed := mustParseURL(t, got)
	if parsed.Host != "localhost:9000" || parsed.Path != "/uniezz-media/chat/1.jpg" {
		t.Errorf("URL = %s, want localhost:9000/uniezz-media/chat/1.jpg", got)
	}
	if parsed.Query().Get("X-Amz-Expires") != "600" {
		t.Errorf("expiry = %q, want 600 seconds", parsed.Query().Get("X-Amz-Expires"))
	}
}

func TestPublicURL(t *testing.T) {
	storage := newTestStorage(t)

	got, err := storage.publicURL("avatars/1.jpg")
	if err != nil {
		t.Fatalf("publicURL() error = %v", err)
	}
	if got != "https://media.uniezz.pl/avatars/1.jpg" {
		t.Errorf("publicURL() = %q", got)
	}

	storage.publicMediaURL = ""
	if _, err := storage.publicURL("avatars/1.jpg"); err == nil {
		t.Error("expected an error when MEDIA_PUBLIC_URL is not configured")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return parsed
}
