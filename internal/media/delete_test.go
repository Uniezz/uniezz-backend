package media

import (
	"context"
	"errors"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type fakeObjects struct {
	objectStore
	deleted   []string
	deleteErr error
	put       map[string][]byte
}

func (f *fakeObjects) deleteObject(_ context.Context, key string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, key)
	return nil
}

func (f *fakeObjects) putObject(_ context.Context, key, _ string, body []byte) error {
	if f.put == nil {
		f.put = map[string][]byte{}
	}
	f.put[key] = body
	return nil
}

func readyMedia(t *testing.T, repo *repository, ownerID uuid.UUID) *mediaRow {
	t.Helper()

	ctx := context.Background()
	item, err := repo.createPending(ctx, ownerID, KindAvatar, objectKey(KindAvatar, "image/jpeg"), "image/jpeg", 10)
	if err != nil {
		t.Fatalf("create pending media: %v", err)
	}
	if err := repo.markReady(ctx, item.ID, ownerID); err != nil {
		t.Fatalf("mark media ready: %v", err)
	}
	return item
}

func status(t *testing.T, repo *repository, id uuid.UUID) string {
	t.Helper()

	var s string
	err := repo.db.Conn(context.Background()).QueryRow(context.Background(), "SELECT status FROM media WHERE id = $1", id).Scan(&s)
	if errors.Is(err, pgx.ErrNoRows) {
		return "gone"
	}
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDelete(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(pool)
	ownerID := createTestUser(t, pool)
	ctx := context.Background()

	markDeleting := func(t *testing.T, m Module, id uuid.UUID) error {
		t.Helper()
		return database.New(pool).WithTx(ctx, func(ctx context.Context) error { return m.MarkDeleting(ctx, id) })
	}

	t.Run("deletes the object, then the row", func(t *testing.T) {
		objects := &fakeObjects{}
		m := Module{repo: repo, storage: objects}
		item := readyMedia(t, repo, ownerID)

		if err := markDeleting(t, m, item.ID); err != nil {
			t.Fatalf("MarkDeleting failed: %v", err)
		}
		if _, err := m.URL(ctx, item.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("media being deleted must not have a URL, got %v", err)
		}
		if err := m.Delete(ctx, item.ID); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		if len(objects.deleted) != 1 || objects.deleted[0] != item.ObjectKey {
			t.Errorf("expected %q deleted, got %v", item.ObjectKey, objects.deleted)
		}
		if s := status(t, repo, item.ID); s != "gone" {
			t.Errorf("expected the row gone, got status %q", s)
		}
	})

	t.Run("a failed object delete keeps the row for the cleanup pass", func(t *testing.T) {
		m := Module{repo: repo, storage: &fakeObjects{deleteErr: errors.New("storage down")}}
		item := readyMedia(t, repo, ownerID)

		if err := markDeleting(t, m, item.ID); err != nil {
			t.Fatalf("MarkDeleting failed: %v", err)
		}
		if err := m.Delete(ctx, item.ID); err == nil {
			t.Fatal("expected the storage error")
		}
		if s := status(t, repo, item.ID); s != "deleting" {
			t.Errorf("expected the row kept as deleting, got %q", s)
		}
	})

	t.Run("ready media is never deleted without being marked first", func(t *testing.T) {
		objects := &fakeObjects{}
		m := Module{repo: repo, storage: objects}
		item := readyMedia(t, repo, ownerID)

		if err := m.Delete(ctx, item.ID); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
		if len(objects.deleted) != 0 || status(t, repo, item.ID) != "ready" {
			t.Error("Delete must only touch media marked as deleting")
		}
	})

	t.Run("pending media cannot be marked for deletion", func(t *testing.T) {
		m := Module{repo: repo, storage: &fakeObjects{}}
		item, err := repo.createPending(ctx, ownerID, KindAvatar, objectKey(KindAvatar, "image/jpeg"), "image/jpeg", 10)
		if err != nil {
			t.Fatal(err)
		}

		if err := markDeleting(t, m, item.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}
