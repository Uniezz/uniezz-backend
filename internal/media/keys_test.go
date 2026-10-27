package media

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestMediaKeys(t *testing.T) {
	tests := []struct {
		name string
		make func(string, string) (string, error)
		id   string
		want string
	}{
		{"avatar", AvatarKey, "user-123", "avatars/user-123/photo.jpg"},
		{"feed", FeedKey, "post-456", "feed/post-456/photo.jpg"},
		{"chat", ChatKey, "thread-789", "chat/thread-789/photo.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.make(tt.id, "photo.jpg")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMediaKeysRejectInvalidParts(t *testing.T) {
	tests := []struct {
		name string
		make func(string, string) (string, error)
		id   string
		file string
	}{
		{"empty id", AvatarKey, "", "photo.jpg"},
		{"id path traversal", FeedKey, "../other", "photo.jpg"},
		{"filename path", ChatKey, "thread-1", "../photo.jpg"},
		{"filename backslash", AvatarKey, "user-1", `..\photo.jpg`},
		{"empty filename", FeedKey, "post-1", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.make(tt.id, tt.file); err == nil {
				t.Fatal("expected an error for invalid key part")
			}
		})
	}
}

func TestNewObjectKey(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		contentType string
		prefix      string
		extension   string
	}{
		{"avatar jpeg", KindAvatar, "image/jpeg", "avatars/", ".jpg"},
		{"feed png", KindFeed, "image/png", "feed/", ".png"},
		{"chat webp", KindChat, "image/webp", "chat/", ".webp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := NewObjectKey(tt.kind, tt.contentType)
			if err != nil {
				t.Fatalf("NewObjectKey() error = %v", err)
			}
			if !strings.HasPrefix(key, tt.prefix) || !strings.HasSuffix(key, tt.extension) {
				t.Fatalf("key %q should start with %q and end with %q", key, tt.prefix, tt.extension)
			}

			id := strings.TrimSuffix(strings.TrimPrefix(key, tt.prefix), tt.extension)
			if _, err := uuid.Parse(id); err != nil {
				t.Errorf("key contains invalid UUID %q: %v", id, err)
			}
		})
	}
}

func TestNewObjectKeyRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		kind        Kind
		contentType string
	}{
		{"unknown kind", Kind("other"), "image/jpeg"},
		{"unsupported content type", KindAvatar, "image/svg+xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewObjectKey(tt.kind, tt.contentType); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}
