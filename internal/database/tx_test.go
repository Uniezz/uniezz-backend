package database_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func setupPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping integration test because DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pool, err := database.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func txID(t *testing.T, ctx context.Context, db database.DB) int64 {
	t.Helper()
	var id int64
	if err := db.Conn(ctx).QueryRow(ctx, "SELECT txid_current()").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestWithTx(t *testing.T) {
	pool := setupPool(t)
	db := database.New(pool)
	ctx := context.Background()

	t.Run("Conn outside WithTx uses the pool", func(t *testing.T) {
		if _, ok := db.Conn(ctx).(*pgxpool.Pool); !ok {
			t.Errorf("expected the pool, got %T", db.Conn(ctx))
		}
	})

	t.Run("Conn inside WithTx uses the transaction, and nested WithTx joins it", func(t *testing.T) {
		err := db.WithTx(ctx, func(ctx context.Context) error {
			if _, ok := db.Conn(ctx).(pgx.Tx); !ok {
				t.Errorf("expected a transaction, got %T", db.Conn(ctx))
			}
			outer := txID(t, ctx, db)
			return database.New(pool).WithTx(ctx, func(ctx context.Context) error {
				if inner := txID(t, ctx, db); inner != outer {
					t.Errorf("nested WithTx opened transaction %d, want %d", inner, outer)
				}
				return nil
			})
		})
		if err != nil {
			t.Fatalf("WithTx failed: %v", err)
		}
	})

	t.Run("an error rolls back every write in the transaction", func(t *testing.T) {
		boom := errors.New("boom")
		marker := time.Now().Nanosecond()
		err := db.WithTx(ctx, func(ctx context.Context) error {
			if _, err := db.Conn(ctx).Exec(ctx, "CREATE TABLE IF NOT EXISTS tx_probe_rollback (n INT)"); err != nil {
				return err
			}
			if _, err := db.Conn(ctx).Exec(ctx, "INSERT INTO tx_probe_rollback VALUES ($1)", marker); err != nil {
				return err
			}
			return boom
		})
		if !errors.Is(err, boom) {
			t.Fatalf("expected boom, got %v", err)
		}

		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('tx_probe_rollback') IS NOT NULL").Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if exists {
			t.Error("the table created in the rolled-back transaction must not exist")
		}
	})
}
