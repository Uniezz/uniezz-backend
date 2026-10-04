package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// newTestLoginCode stores a fresh login code for userID and returns its hash.
func newTestLoginCode(t *testing.T, repo *loginCodeRepository, userID uuid.UUID) []byte {
	t.Helper()

	hash := GenerateSessionToken().Hash()
	if err := repo.create(context.Background(), userID, hash[:]); err != nil {
		t.Fatalf("create failed: %v", err)
	}
	return hash[:]
}

func countLoginCodes(t *testing.T, pool *pgxpool.Pool, codeHash []byte) int {
	t.Helper()

	var n int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM login_codes WHERE code_hash = $1", codeHash).Scan(&n)
	if err != nil {
		t.Fatalf("failed to count login codes: %v", err)
	}
	return n
}

func TestLoginCodeRepository_Integration(t *testing.T) {
	pool := setupTestDB(t)
	repo := newLoginCodeRepository(pool)
	ctx := context.Background()

	t.Run("consume returns the code's user", func(t *testing.T) {
		userID := createTestUser(t, pool)
		hash := newTestLoginCode(t, repo, userID)

		got, err := repo.consume(ctx, hash)
		if err != nil {
			t.Fatalf("consume failed: %v", err)
		}
		if got != userID {
			t.Errorf("expected user %v, got %v", userID, got)
		}
	})

	t.Run("consume works only once", func(t *testing.T) {
		userID := createTestUser(t, pool)
		hash := newTestLoginCode(t, repo, userID)

		if _, err := repo.consume(ctx, hash); err != nil {
			t.Fatalf("first consume failed: %v", err)
		}

		_, err := repo.consume(ctx, hash)
		if !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode on the second consume, got: %v", err)
		}
		if n := countLoginCodes(t, pool, hash); n != 0 {
			t.Errorf("expected the row to be deleted, found %d", n)
		}
	})

	t.Run("unknown code", func(t *testing.T) {
		unknown := GenerateSessionToken().Hash()

		_, err := repo.consume(ctx, unknown[:])
		if !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode, got: %v", err)
		}
	})

	t.Run("expired code is rejected and deleted", func(t *testing.T) {
		userID := createTestUser(t, pool)
		hash := newTestLoginCode(t, repo, userID)
		_, err := pool.Exec(ctx,
			"UPDATE login_codes SET expires_at = NOW() - INTERVAL '1 second' WHERE code_hash = $1", hash)
		if err != nil {
			t.Fatalf("failed to expire the code: %v", err)
		}

		_, err = repo.consume(ctx, hash)
		if !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode for an expired code, got: %v", err)
		}
		if n := countLoginCodes(t, pool, hash); n != 0 {
			t.Errorf("expected the expired row to be deleted, found %d", n)
		}
	})

	t.Run("new code expires in about 60 seconds", func(t *testing.T) {
		userID := createTestUser(t, pool)
		hash := newTestLoginCode(t, repo, userID)

		var ttl time.Duration
		err := pool.QueryRow(ctx,
			"SELECT EXTRACT(EPOCH FROM expires_at - NOW())::float8 * 1e9 FROM login_codes WHERE code_hash = $1",
			hash).Scan(&ttl)
		if err != nil {
			t.Fatalf("failed to read expires_at: %v", err)
		}
		if ttl < 50*time.Second || ttl > 60*time.Second {
			t.Errorf("expected the code to expire in about 60 seconds, got %v", ttl)
		}
	})

	t.Run("deleting the user deletes their codes", func(t *testing.T) {
		userID := createTestUser(t, pool)
		hash := newTestLoginCode(t, repo, userID)

		if _, err := pool.Exec(ctx, "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Fatalf("failed to delete user: %v", err)
		}

		if n := countLoginCodes(t, pool, hash); n != 0 {
			t.Errorf("expected ON DELETE CASCADE to remove the code, found %d", n)
		}
	})

	t.Run("a code for a missing user is rejected", func(t *testing.T) {
		hash := GenerateSessionToken().Hash()

		if err := repo.create(ctx, uuid.New(), hash[:]); err == nil {
			t.Fatal("expected the foreign key to reject the code, got nil")
		}
	})
}
