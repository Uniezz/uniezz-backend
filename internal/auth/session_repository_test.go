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
	repo := NewSessionRepository(database.New(pool))
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

	t.Run("Session of a user being deleted is not returned", func(t *testing.T) {
		deletingID := createTestUser(t, pool)
		token := GenerateSessionToken()
		hash := token.Hash()

		if _, err := repo.CreateSession(ctx, deletingID, hash[:], time.Now().Add(24*time.Hour)); err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}
		if _, err := pool.Exec(ctx, "UPDATE users SET deleted_at = NOW() WHERE id = $1", deletingID); err != nil {
			t.Fatalf("Failed to mark user as deleting: %v", err)
		}

		_, err := repo.GetActiveSessionByHash(ctx, hash[:])
		if !errors.Is(err, ErrSessionNotFound) {
			t.Fatalf("Expected ErrSessionNotFound for a user being deleted, got: %v", err)
		}
	})
}

func TestSessionRepository_GetActiveSessionByHash_Roles(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	repo := NewSessionRepository(database.New(pool))

	session := func(t *testing.T, roles ...Role) *Session {
		t.Helper()
		userID := createTestUser(t, pool)
		for _, role := range roles {
			if _, err := pool.Exec(ctx, "INSERT INTO user_roles (user_id, role) VALUES ($1, $2)", userID, role); err != nil {
				t.Fatalf("grant role: %v", err)
			}
		}
		hash := GenerateSessionToken().Hash()
		if _, err := repo.CreateSession(ctx, userID, hash[:], time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("create session: %v", err)
		}
		s, err := repo.GetActiveSessionByHash(ctx, hash[:])
		if err != nil {
			t.Fatalf("GetActiveSessionByHash failed: %v", err)
		}
		return s
	}

	t.Run("a student has no roles", func(t *testing.T) {
		if s := session(t); len(s.Roles) != 0 {
			t.Errorf("expected no roles, got %v", s.Roles)
		}
	})

	t.Run("every granted role is loaded with the session", func(t *testing.T) {
		s := session(t, RoleModerator, RoleAdmin)
		if len(s.Roles) != 2 || s.Roles[0] != RoleAdmin || s.Roles[1] != RoleModerator {
			t.Errorf("expected [admin moderator], got %v", s.Roles)
		}
	})
}
