package media

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

func TestValidateUpload(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		contentType string
		size        int64
		wantErr     bool
	}{
		{"avatar at limit", KindAvatar, "image/jpeg", 5 << 20, false},
		{"avatar over limit", KindAvatar, "image/jpeg", 5<<20 + 1, true},
		{"feed at limit", KindFeed, "image/webp", 10 << 20, false},
		{"feed over limit", KindFeed, "image/webp", 10<<20 + 1, true},
		{"chat at limit", KindChat, "image/png", 10 << 20, false},
		{"chat over limit", KindChat, "image/png", 10<<20 + 1, true},
		{"rejects SVG", KindAvatar, "image/svg+xml", 1024, true},
		{"rejects unknown kind", Kind("other"), "image/jpeg", 1024, true},
		{"rejects zero size", KindAvatar, "image/jpeg", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUpload(tt.kind, tt.contentType, tt.size)
			if tt.wantErr != errors.Is(err, ErrInvalidUpload) {
				t.Errorf("validateUpload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestObjectKey(t *testing.T) {
	tests := []struct {
		kind        Kind
		contentType string
		prefix      string
		extension   string
	}{
		{KindAvatar, "image/jpeg", "avatars/", ".jpg"},
		{KindFeed, "image/png", "feed/", ".png"},
		{KindChat, "image/webp", "chat/", ".webp"},
	}

	for _, tt := range tests {
		key := objectKey(tt.kind, tt.contentType)

		id, ok := strings.CutPrefix(key, tt.prefix)
		id, ok2 := strings.CutSuffix(id, tt.extension)
		if !ok || !ok2 {
			t.Fatalf("key %q should look like %s<uuid>%s", key, tt.prefix, tt.extension)
		}
		if _, err := uuid.Parse(id); err != nil {
			t.Errorf("key %q contains invalid UUID: %v", key, err)
		}
	}
}

func TestCreateUploadRejectsInvalidInputBeforeStorage(t *testing.T) {
	_, err := Module{}.CreateUpload(context.Background(), uuid.New(), KindAvatar, "image/svg+xml", 1024)
	if !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("CreateUpload() error = %v, want ErrInvalidUpload", err)
	}
}

func TestCreateUpload(t *testing.T) {
	pool := setupTestDB(t)
	ownerID := createTestUser(t, pool)
	module := Module{storage: newTestStorage(t), repo: newRepository(database.New(pool))}

	upload, err := module.CreateUpload(context.Background(), ownerID, KindAvatar, "image/jpeg", 1024)
	if err != nil {
		t.Fatalf("CreateUpload() error = %v", err)
	}

	if host := mustParseURL(t, upload.UploadURL).Host; host != "localhost:9000" {
		t.Errorf("upload URL host = %q, want localhost:9000", host)
	}
	if until := time.Until(upload.ExpiresAt); until < 9*time.Minute || until > 10*time.Minute {
		t.Errorf("ExpiresAt in %v, want about 10 minutes", until)
	}

	item, err := newRepository(database.New(pool)).getOwnedPending(context.Background(), upload.MediaID, ownerID, KindAvatar)
	if err != nil {
		t.Fatalf("get pending media: %v", err)
	}
	if item.SizeBytes != 1024 || item.ContentType != "image/jpeg" {
		t.Errorf("stored size %d and type %q, want 1024 and image/jpeg", item.SizeBytes, item.ContentType)
	}
}
