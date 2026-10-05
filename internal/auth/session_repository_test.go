package auth

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Uniezz/uniezz-backend/internal/database"
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

	testEmail := "test-" + uuid.NewString() + "@pollub.pl"

	var userID uuid.UUID
	query := `
		INSERT INTO users (university_id, email)
		VALUES ($1, $2)
		RETURNING id;
	`
	err := pool.QueryRow(ctx, query, string(UniversityPolLub), testEmail).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM users WHERE id = $1", userID)
	})

	return userID
}

func TestSessionRepository_Integration(t *testing.T) {
	pool := setupTestDB(t)
	repo := NewSessionRepository(pool)
	userID := createTestUser(t, pool)

	ctx := context.Background()

	t.Run("Create and find active session by token hash", func(t *testing.T) {
		token := GenerateSessionToken()

		hash := token.Hash()
		expiresAt := time.Now().Add(24 * time.Hour)

		createdSession, err := repo.CreateSession(ctx, userID, hash[:], expiresAt)
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		if createdSession.UserID != userID {
			t.Fatalf("Expected userID %v, got %v", userID, createdSession.UserID)
		}

		foundSession, err := repo.GetActiveSessionByHash(ctx, hash[:])
		if err != nil {
			t.Fatalf("Expected to find active session, got error: %v", err)
		}

		if foundSession.ID != createdSession.ID {
			t.Errorf("Expected session ID %v, got %v", createdSession.ID, foundSession.ID)
		}
	})

	t.Run("Expired session is not returned", func(t *testing.T) {
		token := GenerateSessionToken()

		hash := token.Hash()
		expiredAt := time.Now().Add(-1 * time.Hour)

		_, err := repo.CreateSession(ctx, userID, hash[:], expiredAt)
		if err != nil {
			t.Fatalf("Failed to insert expired session: %v", err)
		}

		_, err = repo.GetActiveSessionByHash(ctx, hash[:])
		if !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("Expected ErrSessionNotFound for expired session, got: %v", err)
		}
	})

	t.Run("Non-existent token hash returns ErrSessionNotFound", func(t *testing.T) {
		unknownHash := make([]byte, 32)

		_, err := repo.GetActiveSessionByHash(ctx, unknownHash)
		if !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("Expected ErrSessionNotFound for unknown hash, got: %v", err)
		}
	})
}
