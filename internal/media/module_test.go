package media

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateUploadRejectsInvalidInput(t *testing.T) {
	module := Module{}

	tests := []struct {
		name        string
		kind        Kind
		contentType string
		size        int64
	}{
		{"unknown kind", Kind("other"), "image/jpeg", 1024},
		{"unsupported type", KindAvatar, "image/svg+xml", 1024},
		{"size exceeds avatar limit", KindAvatar, "image/jpeg", 5*1024*1024 + 1},
		{"zero size", KindAvatar, "image/jpeg", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := module.CreateUpload(
				context.Background(),
				uuid.New(),
				tt.kind,
				tt.contentType,
				tt.size,
			)
			if err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestCreateUpload(t *testing.T) {
	pool := setupTestDB(t)
	ownerID := createTestUser(t, pool)
	module := Module{
		storage: newTestStorage(),
		repo:    newRepository(pool),
	}

	upload, err := module.CreateUpload(
		context.Background(),
		ownerID,
		KindAvatar,
		"image/jpeg",
		1024,
	)
	if err != nil {
		t.Fatalf("CreateUpload() error = %v", err)
	}
	if upload.MediaID == uuid.Nil {
		t.Fatal("CreateUpload() returned an empty media ID")
	}

	parsedURL, err := url.Parse(upload.UploadURL)
	if err != nil {
		t.Fatalf("parse upload URL: %v", err)
	}
	if parsedURL.Host != "localhost:9000" {
		t.Errorf("upload URL host = %q, want localhost:9000", parsedURL.Host)
	}

	now := time.Now()
	if upload.ExpiresAt.Before(now.Add(9*time.Minute)) ||
		upload.ExpiresAt.After(now.Add(11*time.Minute)) {
		t.Errorf("ExpiresAt = %v, want about 10 minutes from now", upload.ExpiresAt)
	}

	item, err := module.repo.getOwnedPending(
		context.Background(),
		upload.MediaID,
		ownerID,
		KindAvatar,
	)
	if err != nil {
		t.Fatalf("get pending media: %v", err)
	}
	if item.SizeBytes != 1024 || item.ContentType != "image/jpeg" {
		t.Errorf("stored media = size %d, type %q; want size 1024 and image/jpeg",
			item.SizeBytes, item.ContentType)
	}
}

func TestModuleURL(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(pool)
	ownerID := createTestUser(t, pool)

	storage := newTestStorage()
	storage.publicMediaURL = "https://media.uniezz.pl"
	module := Module{
		storage: storage,
		repo:    repo,
	}

	tests := []struct {
		name        string
		kind        Kind
		contentType string
	}{
		{"avatar uses public URL", KindAvatar, "image/jpeg"},
		{"feed uses public URL", KindFeed, "image/png"},
		{"chat uses presigned URL", KindChat, "image/webp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			key, err := NewObjectKey(tt.kind, tt.contentType)
			if err != nil {
				t.Fatalf("create object key: %v", err)
			}

			item, err := repo.createPending(ctx, ownerID, tt.kind, key, tt.contentType, 1024)
			if err != nil {
				t.Fatalf("create pending media: %v", err)
			}
			if err := repo.markReady(ctx, item.ID, ownerID); err != nil {
				t.Fatalf("mark media ready: %v", err)
			}

			got, err := module.URL(ctx, item.ID)
			if err != nil {
				t.Fatalf("Module.URL() error = %v", err)
			}

			if tt.kind == KindChat {
				parsed, err := url.Parse(got)
				if err != nil {
					t.Fatalf("parse presigned URL: %v", err)
				}
				if parsed.Host != "localhost:9000" {
					t.Errorf("URL host = %q, want localhost:9000", parsed.Host)
				}
				if parsed.Query().Get("X-Amz-Expires") != "600" {
					t.Errorf("URL expiry = %q, want 600 seconds", parsed.Query().Get("X-Amz-Expires"))
				}
				return
			}

			want := "https://media.uniezz.pl/" + key
			if got != want {
				t.Errorf("Module.URL() = %q, want %q", got, want)
			}
		})
	}
}
