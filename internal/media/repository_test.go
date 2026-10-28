package media

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
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
	repo := newRepository(pool)
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

	if err := repo.markReady(ctx, created.ID, ownerID); err != nil {
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
	if err := repo.markReady(ctx, created.ID, ownerID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second markReady: error = %v, want ErrNotFound", err)
	}
}
