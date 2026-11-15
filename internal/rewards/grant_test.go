package rewards

import (
	"context"
	"errors"
	"os"
	"sync"
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

func newUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(context.Background(), `INSERT INTO users (university_id, email) VALUES ('umcs', $1) RETURNING id`,
		"rewards-"+uuid.NewString()+"@example.test").Scan(&id)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id) })
	return id
}

func grantRows(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM coin_grants WHERE user_id = $1", userID).Scan(&n); err != nil {
		t.Fatalf("count grants: %v", err)
	}
	return n
}

func TestGrant(t *testing.T) {
	pool := setupTestDB(t)
	m := New(pool)
	ctx := context.Background()

	t.Run("a student with no grants has a balance of zero", func(t *testing.T) {
		if got, err := m.Balance(ctx, newUser(t, pool)); err != nil || got != 0 {
			t.Errorf("balance = %d, %v; want 0", got, err)
		}
	})

	t.Run("the same action pays once and reports it", func(t *testing.T) {
		user := newUser(t, pool)
		for i, want := range []bool{true, false} {
			paid, err := m.Grant(ctx, user, ReasonWelcome, nil, nil)
			if err != nil || paid != want {
				t.Fatalf("grant %d: paid %v, %v; want %v", i, paid, err, want)
			}
		}
		if got, _ := m.Balance(ctx, user); got != 100 || grantRows(t, pool, user) != 1 {
			t.Errorf("balance %d, rows %d; want 100, 1", got, grantRows(t, pool, user))
		}
	})

	t.Run("each liker of a post pays the author once and a new post pays again", func(t *testing.T) {
		author, post, otherPost := newUser(t, pool), uuid.New(), uuid.New()
		first, second := newUser(t, pool), newUser(t, pool)
		grants := []struct{ ref, actor *uuid.UUID }{{&post, &first}, {&post, &first}, {&post, &second}, {&otherPost, &first}}
		for _, g := range grants {
			if _, err := m.Grant(ctx, author, ReasonLikeReceived, g.ref, g.actor); err != nil {
				t.Fatalf("grant: %v", err)
			}
		}
		if got, _ := m.Balance(ctx, author); got != 3 {
			t.Errorf("balance = %d, want 3", got)
		}
	})

	t.Run("concurrent grants of one action pay once", func(t *testing.T) {
		user, post := newUser(t, pool), uuid.New()
		var wg sync.WaitGroup
		errs := make([]error, 10)
		for i := range errs {
			wg.Go(func() { _, errs[i] = m.Grant(ctx, user, ReasonComment, &post, nil) })
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			t.Fatalf("grant: %v", err)
		}
		if got, _ := m.Balance(ctx, user); got != 1 || grantRows(t, pool, user) != 1 {
			t.Errorf("balance %d, rows %d; want 1, 1", got, grantRows(t, pool, user))
		}
	})

	t.Run("a grant inside a rolled-back transaction pays nothing", func(t *testing.T) {
		user := newUser(t, pool)
		rollback := errors.New("rollback")
		err := database.New(pool).WithTx(ctx, func(ctx context.Context) error {
			if _, err := m.Grant(ctx, user, ReasonBirthDate, nil, nil); err != nil {
				return err
			}
			return rollback
		})
		if !errors.Is(err, rollback) {
			t.Fatalf("tx err = %v", err)
		}
		if got, _ := m.Balance(ctx, user); got != 0 || grantRows(t, pool, user) != 0 {
			t.Errorf("balance %d, rows %d; want 0, 0", got, grantRows(t, pool, user))
		}
	})

	t.Run("an unknown reason is an error and pays nothing", func(t *testing.T) {
		user := newUser(t, pool)
		if _, err := m.Grant(ctx, user, Reason("bogus"), nil, nil); err == nil {
			t.Error("want an error")
		}
		if n := grantRows(t, pool, user); n != 0 {
			t.Errorf("rows = %d, want 0", n)
		}
	})
}
