package media

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestCleanup(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(pool)
	ownerID := createTestUser(t, pool)
	ctx := context.Background()

	pending := func(t *testing.T, age time.Duration) *mediaRow {
		t.Helper()
		item, err := repo.createPending(ctx, ownerID, KindAvatar, objectKey(KindAvatar, "image/jpeg"), "image/jpeg", 10)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, "UPDATE media SET created_at = NOW() - $2::interval WHERE id = $1", item.ID, age); err != nil {
			t.Fatal(err)
		}
		return item
	}
	deleting := func(t *testing.T) *mediaRow {
		t.Helper()
		item := readyMedia(t, repo, ownerID)
		if err := repo.markDeleting(ctx, item.ID); err != nil {
			t.Fatal(err)
		}
		return item
	}

	t.Run("removes abandoned uploads and leftover deletions, keeps the rest", func(t *testing.T) {
		abandoned := pending(t, 2*time.Hour)
		fresh := pending(t, 5*time.Minute)
		leftover := deleting(t)
		ready := readyMedia(t, repo, ownerID)

		objects := &fakeObjects{}
		if err := (Module{repo: repo, storage: objects}).cleanup(ctx, CleanupArgs{}); err != nil {
			t.Fatalf("cleanup failed: %v", err)
		}

		for _, gone := range []*mediaRow{abandoned, leftover} {
			if s := status(t, repo, gone.ID); s != "gone" {
				t.Errorf("expected %v gone, got %q", gone.ID, s)
			}
			if !slices.Contains(objects.deleted, gone.ObjectKey) {
				t.Errorf("expected object %q deleted", gone.ObjectKey)
			}
		}
		if s := status(t, repo, fresh.ID); s != "pending" {
			t.Errorf("an upload still in progress must stay pending, got %q", s)
		}
		if s := status(t, repo, ready.ID); s != "ready" {
			t.Errorf("ready media must stay, got %q", s)
		}
	})

	t.Run("a storage failure keeps the rows for the next run and fails the job", func(t *testing.T) {
		leftover := deleting(t)

		err := (Module{repo: repo, storage: &fakeObjects{deleteErr: errors.New("storage down")}}).cleanup(ctx, CleanupArgs{})
		if err == nil {
			t.Fatal("expected the storage error so the job retries")
		}
		if s := status(t, repo, leftover.ID); s != "deleting" {
			t.Errorf("expected the row kept as deleting, got %q", s)
		}
	})

	t.Run("an abandoned upload can no longer be claimed", func(t *testing.T) {
		abandoned := pending(t, 2*time.Hour)
		if err := repo.markAbandonedDeleting(ctx, abandonedUploadAge); err != nil {
			t.Fatal(err)
		}
		if _, err := repo.getOwnedPending(ctx, abandoned.ID, ownerID, KindAvatar); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}
