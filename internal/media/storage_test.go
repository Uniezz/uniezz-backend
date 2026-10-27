package media

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func newTestStorage() *Storage {
	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("test-key", "test-secret", ""),
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("http://minio:9000")
		options.UsePathStyle = true
	})
	presignClient := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String("http://localhost:9000")
		options.UsePathStyle = true
	})

	return &Storage{
		client:  client,
		presign: s3.NewPresignClient(presignClient),
		bucket:  "uniezz-media",
	}
}

func TestPublicEndpointFallsBackToS3Endpoint(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		expected string
	}{
		{
			name: "uses configured public endpoint",
			cfg: &config.Config{
				AWSEndpoint:       "http://minio:9000",
				AWSPublicEndpoint: "http://localhost:9000",
			},
			expected: "http://localhost:9000",
		},
		{
			name:     "falls back to S3 endpoint",
			cfg:      &config.Config{AWSEndpoint: "https://s3.example.com"},
			expected: "https://s3.example.com",
		},
		{
			name:     "uses default AWS endpoint when both are empty",
			cfg:      &config.Config{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := publicEndpoint(tt.cfg); got != tt.expected {
				t.Errorf("publicEndpoint() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestPresignPutURL(t *testing.T) {
	storage := newTestStorage()
	got, err := storage.PresignPutURL(
		context.Background(),
		"avatars/user-1/photo.jpg",
		KindAvatar,
		"image/jpeg",
		1024,
	)
	if err != nil {
		t.Fatalf("PresignPutURL() error = %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	if parsed.Path != "/uniezz-media/avatars/user-1/photo.jpg" {
		t.Errorf("URL path = %q", parsed.Path)
	}
	if parsed.Host != "localhost:9000" {
		t.Errorf("URL host = %q, want localhost:9000", parsed.Host)
	}
	if parsed.Query().Get("X-Amz-Expires") != "600" {
		t.Errorf("URL expiry = %q, want 600 seconds", parsed.Query().Get("X-Amz-Expires"))
	}
	if !strings.Contains(parsed.Query().Get("X-Amz-SignedHeaders"), "content-type") {
		t.Errorf("Content-Type is not included in signed headers")
	}
	if !strings.Contains(parsed.Query().Get("X-Amz-SignedHeaders"), "content-length") {
		t.Errorf("Content-Length is not included in signed headers")
	}
}

func TestPresignGetURL(t *testing.T) {
	storage := newTestStorage()
	got, err := storage.PresignGetURL(
		context.Background(),
		"feed/post-1/photo.jpg",
	)
	if err != nil {
		t.Fatalf("PresignGetURL() error = %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse presigned URL: %v", err)
	}
	if parsed.Path != "/uniezz-media/feed/post-1/photo.jpg" {
		t.Errorf("URL path = %q", parsed.Path)
	}
	if parsed.Host != "localhost:9000" {
		t.Errorf("URL host = %q, want localhost:9000", parsed.Host)
	}
	if parsed.Query().Get("X-Amz-Expires") != "600" {
		t.Errorf("URL expiry = %q, want 600 seconds", parsed.Query().Get("X-Amz-Expires"))
	}
}

func TestPresignRejectsInvalidInput(t *testing.T) {
	storage := newTestStorage()
	ctx := context.Background()

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "PUT empty key",
			run: func() error {
				_, err := storage.PresignPutURL(ctx, "", KindAvatar, "image/jpeg", 1024)
				return err
			},
		},
		{
			name: "PUT empty content type",
			run: func() error {
				_, err := storage.PresignPutURL(ctx, "photo.jpg", KindAvatar, "", 1024)
				return err
			},
		},
		{
			name: "GET empty key",
			run: func() error {
				_, err := storage.PresignGetURL(ctx, "")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestValidateUpload(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		contentType string
		size        int64
		wantError   bool
	}{
		{"avatar at limit", KindAvatar, "image/jpeg", 5 * 1024 * 1024, false},
		{"avatar over limit", KindAvatar, "image/jpeg", 5*1024*1024 + 1, true},
		{"feed at limit", KindFeed, "image/webp", 10 * 1024 * 1024, false},
		{"feed over limit", KindFeed, "image/webp", 10*1024*1024 + 1, true},
		{"chat at limit", KindChat, "image/png", 10 * 1024 * 1024, false},
		{"chat over limit", KindChat, "image/png", 10*1024*1024 + 1, true},
		{"reject SVG", KindAvatar, "image/svg+xml", 1024, true},
		{"reject unknown kind", Kind("other"), "image/jpeg", 1024, true},
		{"reject zero size", KindAvatar, "image/jpeg", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpload(tt.kind, tt.contentType, tt.size)
			if (err != nil) != tt.wantError {
				t.Errorf("validateUpload() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestPublicURL(t *testing.T) {
	storage := newTestStorage()
	storage.publicMediaURL = "https://media.uniezz.pl/"

	got, err := storage.PublicURL("avatars/123.jpg")
	if err != nil {
		t.Fatalf("PublicURL() error = %v", err)
	}
	if got != "https://media.uniezz.pl/avatars/123.jpg" {
		t.Errorf("PublicURL() = %q", got)
	}
}

func TestPublicURLRejectsMissingURLAndInvalidKeys(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		key     string
	}{
		{"missing public URL", "", "avatars/123.jpg"},
		{"empty key", "https://media.uniezz.pl", ""},
		{"absolute key", "https://media.uniezz.pl", "/avatars/123.jpg"},
		{"traversal key", "https://media.uniezz.pl", "avatars/../private.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage := newTestStorage()
			storage.publicMediaURL = tt.baseURL

			if _, err := storage.PublicURL(tt.key); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
