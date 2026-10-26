package ratelimit

import (
	"context"
	"errors"
	"os"
	"sync"
	"sync/atomic"
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
	t.Cleanup(pool.Close)
	return pool
}

func TestLimiter_Integration(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	t.Run("allows up to the limit, then refuses", func(t *testing.T) {
		l := New(pool, "test", 3, time.Hour)
		key := uuid.NewString()

		for i := range 3 {
			if err := l.Allow(ctx, key); err != nil {
				t.Fatalf("call %d: expected allowed, got: %v", i+1, err)
			}
		}
		if err := l.Allow(ctx, key); !errors.Is(err, ErrLimited) {
			t.Fatalf("expected ErrLimited on call 4, got: %v", err)
		}
	})

	t.Run("keys are counted separately", func(t *testing.T) {
		l := New(pool, "test", 1, time.Hour)
		a, b := uuid.NewString(), uuid.NewString()

		if err := l.Allow(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := l.Allow(ctx, b); err != nil {
			t.Fatalf("expected a different key to be allowed, got: %v", err)
		}
	})

	t.Run("limiters with different names do not share counts", func(t *testing.T) {
		key := uuid.NewString()

		if err := New(pool, "first", 1, time.Hour).Allow(ctx, key); err != nil {
			t.Fatal(err)
		}
		if err := New(pool, "second", 1, time.Hour).Allow(ctx, key); err != nil {
			t.Fatalf("expected another limiter's count to be separate, got: %v", err)
		}
	})

	t.Run("a new window starts a new count", func(t *testing.T) {
		l := New(pool, "test", 1, time.Hour)
		key := uuid.NewString()

		if err := l.Allow(ctx, key); err != nil {
			t.Fatal(err)
		}
		_, err := pool.Exec(ctx,
			"UPDATE rate_limits SET window_start = window_start - INTERVAL '1 hour' WHERE key = $1", "test:"+key)
		if err != nil {
			t.Fatal(err)
		}
		if err := l.Allow(ctx, key); err != nil {
			t.Fatalf("expected the next window to allow again, got: %v", err)
		}
	})

	t.Run("parallel calls never exceed the limit", func(t *testing.T) {
		l := New(pool, "test", 5, time.Hour)
		key := uuid.NewString()

		var wg sync.WaitGroup
		var allowed atomic.Int32
		for range 30 {
			wg.Go(func() {
				if l.Allow(ctx, key) == nil {
					allowed.Add(1)
				}
			})
		}
		wg.Wait()

		if allowed.Load() != 5 {
			t.Errorf("expected exactly 5 allowed calls, got %d", allowed.Load())
		}
	})
}
