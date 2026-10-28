package media

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestURL(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(pool)
	ownerID := createTestUser(t, pool)
	module := Module{storage: newTestStorage(t), repo: repo}
	ctx := context.Background()

	tests := []struct {
		name       string
		kind       Kind
		wantPublic bool
	}{
		{"avatar uses public URL", KindAvatar, true},
		{"feed uses public URL", KindFeed, true},
		{"chat uses presigned URL", KindChat, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := objectKey(tt.kind, "image/jpeg")
			item, err := repo.createPending(ctx, ownerID, tt.kind, key, "image/jpeg", 1024)
			if err != nil {
				t.Fatalf("create pending media: %v", err)
			}
			if err := repo.markReady(ctx, item.ID, ownerID); err != nil {
				t.Fatalf("mark media ready: %v", err)
			}

			got, err := module.URL(ctx, item.ID)
			if err != nil {
				t.Fatalf("URL() error = %v", err)
			}

			if tt.wantPublic {
				if want := "https://media.uniezz.pl/" + key; got != want {
					t.Errorf("URL() = %q, want %q", got, want)
				}
				return
			}
			if host := mustParseURL(t, got).Host; host != "localhost:9000" {
				t.Errorf("presigned URL host = %q, want localhost:9000", host)
			}
		})
	}
}

func TestURLOfUnknownMedia(t *testing.T) {
	pool := setupTestDB(t)
	module := Module{storage: newTestStorage(t), repo: newRepository(pool)}

	if _, err := module.URL(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("URL() error = %v, want ErrNotFound", err)
	}
}
