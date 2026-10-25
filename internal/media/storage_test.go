package media

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

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
		"image/jpeg",
		15*time.Minute,
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
	if parsed.Query().Get("X-Amz-Expires") != "900" {
		t.Errorf("URL expiry = %q, want 900 seconds", parsed.Query().Get("X-Amz-Expires"))
	}
	if !strings.Contains(parsed.Query().Get("X-Amz-SignedHeaders"), "content-type") {
		t.Errorf("Content-Type is not included in signed headers")
	}
}

func TestPresignGetURL(t *testing.T) {
	storage := newTestStorage()
	got, err := storage.PresignGetURL(
		context.Background(),
		"feed/post-1/photo.jpg",
		10*time.Minute,
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
				_, err := storage.PresignPutURL(ctx, "", "image/jpeg", time.Minute)
				return err
			},
		},
		{
			name: "PUT empty content type",
			run: func() error {
				_, err := storage.PresignPutURL(ctx, "photo.jpg", "", time.Minute)
				return err
			},
		},
		{
			name: "GET empty key",
			run: func() error {
				_, err := storage.PresignGetURL(ctx, "", time.Minute)
				return err
			},
		},
		{
			name: "zero expiry",
			run: func() error {
				_, err := storage.PresignGetURL(ctx, "photo.jpg", 0)
				return err
			},
		},
		{
			name: "expiry over seven days",
			run: func() error {
				_, err := storage.PresignGetURL(ctx, "photo.jpg", 8*24*time.Hour)
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
