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

func TestURLOf(t *testing.T) {
	module := Module{storage: newTestStorage(t)}
	ctx := context.Background()

	got, err := module.URLOf(ctx, Ref{Kind: KindAvatar, ObjectKey: "avatars/a.jpg"})
	if err != nil || got != "https://media.uniezz.pl/avatars/a.jpg" {
		t.Errorf("URLOf(avatar) = %q, %v; want the public URL", got, err)
	}

	got, err = module.URLOf(ctx, Ref{Kind: KindChat, ObjectKey: "chat/a.jpg"})
	if err != nil {
		t.Fatalf("URLOf(chat) error = %v", err)
	}
	if host := mustParseURL(t, got).Host; host != "localhost:9000" {
		t.Errorf("presigned URL host = %q, want localhost:9000", host)
	}
}

func TestReadyMediaView(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(database.New(pool))
	ownerID := createTestUser(t, pool)
	ctx := context.Background()

	ready := readyMedia(t, repo, ownerID)
	pending, err := repo.createPending(ctx, ownerID, KindAvatar, objectKey(KindAvatar, "image/jpeg"), "image/jpeg", 10)
	if err != nil {
		t.Fatal(err)
	}
	deleting := readyMedia(t, repo, ownerID)
	if err := repo.markDeleting(ctx, deleting.ID); err != nil {
		t.Fatal(err)
	}

	var kind Kind
	var key string
	err = pool.QueryRow(ctx, "SELECT kind, object_key FROM ready_media WHERE id = $1", ready.ID).Scan(&kind, &key)
	if err != nil || kind != KindAvatar || key != ready.ObjectKey {
		t.Errorf("ready media = %q %q %v, want %q %q", kind, key, err, KindAvatar, ready.ObjectKey)
	}

	var hidden int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM ready_media WHERE id = ANY($1)", []uuid.UUID{pending.ID, deleting.ID}).Scan(&hidden); err != nil {
		t.Fatal(err)
	}
	if hidden != 0 {
		t.Errorf("pending and deleting media must not be in ready_media, found %d", hidden)
	}
}
