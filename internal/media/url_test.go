package media

import (
	"context"
	"errors"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

func TestURL(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(database.New(pool))
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
	module := Module{storage: newTestStorage(t), repo: newRepository(database.New(pool))}

	if _, err := module.URL(context.Background(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("URL() error = %v, want ErrNotFound", err)
	}
}

func TestURLs(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(database.New(pool))
	ownerID := createTestUser(t, pool)
	module := Module{storage: newTestStorage(t), repo: repo}
	ctx := context.Background()

	ready := readyMedia(t, repo, ownerID)
	pending, err := repo.createPending(ctx, ownerID, KindAvatar, objectKey(KindAvatar, "image/jpeg"), "image/jpeg", 10)
	if err != nil {
		t.Fatal(err)
	}

	urls, err := module.URLs(ctx, []uuid.UUID{ready.ID, pending.ID, uuid.New()})
	if err != nil {
		t.Fatalf("URLs() error = %v", err)
	}
	if want := "https://media.uniezz.pl/" + ready.ObjectKey; len(urls) != 1 || urls[ready.ID] != want {
		t.Errorf("URLs() = %v, want only %s -> %s", urls, ready.ID, want)
	}

	if urls, err := module.URLs(ctx, nil); err != nil || len(urls) != 0 {
		t.Errorf("URLs(nil) = %v, %v; want an empty map", urls, err)
	}
}
