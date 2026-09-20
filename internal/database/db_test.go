package database_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
)

func TestNewPool_InvalidURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	invalidURL := "postgres://invalid-url"

	pool, err := database.NewPool(ctx, invalidURL)
	if err == nil {
		if pool != nil {
			pool.Close()
		}
		t.Fatal("Expected error when creating pool with invalid URL, got nil")
	}
}

func TestNewPool_Integration_Success(t *testing.T) {
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
	defer pool.Close()

	var pingResult int
	err = pool.QueryRow(ctx, "SELECT 1").Scan(&pingResult)
	if err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	if pingResult != 1 {
		t.Fatalf("Expected ping result to be 1, got %d", pingResult)
	}
}
