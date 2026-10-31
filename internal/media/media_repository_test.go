package media

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping database integration test in short mode")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping database integration test because DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to create database pool: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func createTestUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	email := "media-test-" + uuid.NewString() + "@example.test"

	var userID uuid.UUID
	err := pool.QueryRow(ctx, `
        INSERT INTO users (university_id, email)
        VALUES ('umcs', $1)
        RETURNING id;
    `, email).Scan(&userID)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()

		if _, err := pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete test user: %v", err)
		}
	})

	return userID
}

func TestRepositoryLifecycle(t *testing.T) {
	pool := setupTestDB(t)
	repo := newRepository(database.New(pool))
	ownerID := createTestUser(t, pool)

	ctx := context.Background()
	objectKey := "avatars/" + uuid.NewString() + ".jpg"

	created, err := repo.createPending(
		ctx,
		ownerID,
		KindAvatar,
		objectKey,
		"image/jpeg",
		1024,
	)
	if err != nil {
		t.Fatalf("create pending media: %v", err)
	}
	if created.Status != "pending" {
		t.Fatalf("status = %q, want pending", created.Status)
	}
	if created.OwnerUserID != ownerID {
		t.Errorf("owner ID = %v, want %v", created.OwnerUserID, ownerID)
	}
	if created.ObjectKey != objectKey {
		t.Errorf("object key = %q, want %q", created.ObjectKey, objectKey)
	}

	found, err := repo.getOwnedPending(ctx, created.ID, ownerID, KindAvatar)
	if err != nil {
		t.Fatalf("get pending media: %v", err)
	}
	if found.ID != created.ID {
		t.Errorf("media ID = %v, want %v", found.ID, created.ID)
	}

	if _, err := repo.getOwnedPending(ctx, created.ID, uuid.New(), KindAvatar); !errors.Is(err, ErrNotFound) {
		t.Fatalf("different owner: error = %v, want ErrNotFound", err)
	}

	if err := repo.markReady(ctx, created.ID, ownerID, KindFeed); !errors.Is(err, ErrNotFound) {
		t.Fatalf("markReady with another kind: error = %v, want ErrNotFound", err)
	}
	if err := repo.markReady(ctx, created.ID, ownerID, created.Kind); err != nil {
		t.Fatalf("mark media ready: %v", err)
	}

	ready, err := repo.getReady(ctx, created.ID)
	if err != nil {
		t.Fatalf("get ready media: %v", err)
	}
	if ready.Status != "ready" {
		t.Errorf("status = %q, want ready", ready.Status)
	}

	if _, err := repo.getOwnedPending(ctx, created.ID, ownerID, KindAvatar); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ready media returned as pending: error = %v, want ErrNotFound", err)
	}
	if err := repo.markReady(ctx, created.ID, ownerID, created.Kind); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second markReady: error = %v, want ErrNotFound", err)
	}
}

func TestClaimVersusCleanup(t *testing.T) {
	pool := setupTestDB(t)
	db := database.New(pool)
	repo := newRepository(db)
	ctx := context.Background()
	ownerID := createTestUser(t, pool)

	abandoned := func(t *testing.T) uuid.UUID {
		t.Helper()
		item, err := repo.createPending(ctx, ownerID, KindFeed, "feed/"+uuid.NewString()+".jpg", "image/jpeg", 10)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE media SET created_at = NOW() - INTERVAL '2 hours' WHERE id = $1`, item.ID); err != nil {
			t.Fatal(err)
		}
		return item.ID
	}
	status := func(t *testing.T, id uuid.UUID) string {
		t.Helper()
		var s string
		if err := pool.QueryRow(ctx, `SELECT status FROM media WHERE id = $1`, id).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("claim holding the row wins over cleanup", func(t *testing.T) {
		id := abandoned(t)
		claimed := make(chan struct{})
		release := make(chan struct{})
		var releaseOnce sync.Once
		releaseClaim := func() { releaseOnce.Do(func() { close(release) }) }
		t.Cleanup(releaseClaim)
		done := make(chan error, 1)

		go func() {
			done <- db.WithTx(ctx, func(ctx context.Context) error {
				if err := repo.markReady(ctx, id, ownerID, KindFeed); err != nil {
					return err
				}
				close(claimed)
				<-release
				return nil
			})
		}()
		select {
		case <-claimed:
		case err := <-done:
			t.Fatalf("claim: %v", err)
		}

		cleanupDone := make(chan error, 1)
		go func() { cleanupDone <- repo.markAbandonedDeleting(ctx, time.Hour) }()
		waitForLockWait(t, pool, "UPDATE media SET status = 'deleting'")
		releaseClaim()

		if err := <-done; err != nil {
			t.Fatalf("claim: %v", err)
		}
		if err := <-cleanupDone; err != nil {
			t.Fatalf("cleanup: %v", err)
		}
		if got := status(t, id); got != "ready" {
			t.Errorf("status = %q, want ready", got)
		}
	})

	t.Run("cleanup first makes the claim fail", func(t *testing.T) {
		id := abandoned(t)
		if err := repo.markAbandonedDeleting(ctx, time.Hour); err != nil {
			t.Fatal(err)
		}
		if err := repo.markReady(ctx, id, ownerID, KindFeed); !errors.Is(err, ErrNotFound) {
			t.Fatalf("markReady after cleanup: error = %v, want ErrNotFound", err)
		}
		if got := status(t, id); got != "deleting" {
			t.Errorf("status = %q, want deleting", got)
		}
	})

	t.Run("rolled back claim leaves media for cleanup", func(t *testing.T) {
		id := abandoned(t)
		rollback := errors.New("rollback")
		err := db.WithTx(ctx, func(ctx context.Context) error {
			if err := repo.markReady(ctx, id, ownerID, KindFeed); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatalf("WithTx error = %v", err)
		}
		if got := status(t, id); got != "pending" {
			t.Errorf("status = %q, want pending", got)
		}
	})
}

func waitForLockWait(t *testing.T, _ *pgxpool.Pool, queryPart string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Its own connection: the waiting requests may hold every pooled one.
	conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	for ctx.Err() == nil {
		var waiting bool
		err := conn.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE wait_event_type = 'Lock'
				  AND regexp_replace(query, '\s+', ' ', 'g') LIKE '%' || $1 || '%'
			)`, queryPart).Scan(&waiting)
		if err != nil {
			break
		}
		if waiting {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no query containing %q waited for a lock", queryPart)
}

func TestAbandonedUploadsOutliveUploadURLs(t *testing.T) {
	if abandonedUploadAge <= uploadURLTTL {
		t.Fatalf("cleanup age %v must exceed upload URL lifetime %v, or a still valid URL could recreate a deleted object", abandonedUploadAge, uploadURLTTL)
	}
}
