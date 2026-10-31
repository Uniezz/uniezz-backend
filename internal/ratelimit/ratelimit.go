// Package ratelimit counts actions per key in fixed time windows stored in PostgreSQL.
package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrLimited = errors.New("rate limit exceeded")

type Limiter struct {
	db     database.DB
	name   string
	limit  int
	window time.Duration
}

func New(db *pgxpool.Pool, name string, limit int, window time.Duration) *Limiter {
	return &Limiter{db: database.New(db), name: name, limit: limit, window: window}
}

// Allow counts one action. Inside a transaction the count is rolled back with
// it, so an action that fails later does not use up the limit.
func (l *Limiter) Allow(ctx context.Context, key string) error {
	const query = `
		INSERT INTO rate_limits (key, window_start, count)
		VALUES ($1, to_timestamp(floor(extract(epoch FROM NOW()) / $2) * $2), 1)
		ON CONFLICT (key, window_start) DO UPDATE SET count = rate_limits.count + 1
		RETURNING count;
	`

	var count int
	if err := l.db.Conn(ctx).QueryRow(ctx, query, l.name+":"+key, l.window.Seconds()).Scan(&count); err != nil {
		return fmt.Errorf("ratelimit: count %s: %w", l.name, err)
	}
	if count > l.limit {
		return ErrLimited
	}
	return nil
}
