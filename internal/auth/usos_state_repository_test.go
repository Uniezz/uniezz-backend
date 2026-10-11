package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Uniezz/uniezz-backend/internal/usos"
)

// newTestUsosState returns a state with a unique request token and deletes
// its row when the test ends, whether or not take already did.
func newTestUsosState(t *testing.T, pool *pgxpool.Pool, platform Platform) usosState {
	t.Helper()

	token := "test-" + uuid.NewString()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM oauth_states WHERE request_token = $1", token)
	})

	return usosState{
		RequestToken: usos.RequestToken{Token: token, Secret: "secret-" + uuid.NewString()},
		UniversityID: UniversityUMCS,
		Platform:     platform,
	}
}

func countUsosStates(t *testing.T, pool *pgxpool.Pool, requestToken string) int {
	t.Helper()

	var n int
	err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM oauth_states WHERE request_token = $1", requestToken).Scan(&n)
	if err != nil {
		t.Fatalf("failed to count states: %v", err)
	}
	return n
}

func TestUsosStateRepository_Integration(t *testing.T) {
	pool := setupTestDB(t)
	repo := newUsosStateRepository(pool)
	ctx := context.Background()

	t.Run("take returns what save stored", func(t *testing.T) {
		for _, platform := range []Platform{PlatformWeb, PlatformMobile} {
			want := newTestUsosState(t, pool, platform)

			if err := repo.save(ctx, want); err != nil {
				t.Fatalf("save failed: %v", err)
			}

			got, err := repo.take(ctx, want.RequestToken.Token)
			if err != nil {
				t.Fatalf("take failed: %v", err)
			}
			if got != want {
				t.Errorf("state mismatch:\nexpected %+v\ngot      %+v", want, got)
			}
		}
	})

	t.Run("take works only once", func(t *testing.T) {
		s := newTestUsosState(t, pool, PlatformWeb)
		if err := repo.save(ctx, s); err != nil {
			t.Fatalf("save failed: %v", err)
		}

		if _, err := repo.take(ctx, s.RequestToken.Token); err != nil {
			t.Fatalf("first take failed: %v", err)
		}

		_, err := repo.take(ctx, s.RequestToken.Token)
		if !errors.Is(err, ErrStateNotFound) {
			t.Fatalf("expected ErrStateNotFound on the second take, got: %v", err)
		}
		if n := countUsosStates(t, pool, s.RequestToken.Token); n != 0 {
			t.Errorf("expected the row to be deleted, found %d", n)
		}
	})

	t.Run("unknown request token", func(t *testing.T) {
		_, err := repo.take(ctx, "test-never-saved-"+uuid.NewString())
		if !errors.Is(err, ErrStateNotFound) {
			t.Fatalf("expected ErrStateNotFound, got: %v", err)
		}
	})

	t.Run("expired state is rejected and deleted", func(t *testing.T) {
		s := newTestUsosState(t, pool, PlatformWeb)
		if err := repo.save(ctx, s); err != nil {
			t.Fatalf("save failed: %v", err)
		}
		_, err := pool.Exec(ctx,
			"UPDATE oauth_states SET expires_at = NOW() - INTERVAL '1 second' WHERE request_token = $1",
			s.RequestToken.Token)
		if err != nil {
			t.Fatalf("failed to expire the state: %v", err)
		}

		_, err = repo.take(ctx, s.RequestToken.Token)
		if !errors.Is(err, ErrStateNotFound) {
			t.Fatalf("expected ErrStateNotFound for an expired state, got: %v", err)
		}
		if n := countUsosStates(t, pool, s.RequestToken.Token); n != 0 {
			t.Errorf("expected the expired row to be deleted, found %d", n)
		}
	})

	t.Run("new state expires in about 10 minutes", func(t *testing.T) {
		s := newTestUsosState(t, pool, PlatformWeb)
		if err := repo.save(ctx, s); err != nil {
			t.Fatalf("save failed: %v", err)
		}

		var ttl time.Duration
		err := pool.QueryRow(ctx,
			"SELECT EXTRACT(EPOCH FROM expires_at - NOW())::float8 * 1e9 FROM oauth_states WHERE request_token = $1",
			s.RequestToken.Token).Scan(&ttl)
		if err != nil {
			t.Fatalf("failed to read expires_at: %v", err)
		}
		if ttl < 9*time.Minute || ttl > 10*time.Minute {
			t.Errorf("expected the state to expire in about 10 minutes, got %v", ttl)
		}
	})

	t.Run("saving the same request token twice fails", func(t *testing.T) {
		s := newTestUsosState(t, pool, PlatformWeb)
		if err := repo.save(ctx, s); err != nil {
			t.Fatalf("first save failed: %v", err)
		}

		if err := repo.save(ctx, s); err == nil {
			t.Fatal("expected an error for a duplicate request token, got nil")
		}
	})

	t.Run("database rejects an unknown platform", func(t *testing.T) {
		s := newTestUsosState(t, pool, Platform("desktop"))

		if err := repo.save(ctx, s); err == nil {
			t.Fatal("expected the platform CHECK constraint to reject the row, got nil")
		}
	})
}
