// Package jobs runs durable background tasks stored in PostgreSQL. A feature
// module defines a typed task, registers its handler at startup and enqueues
// jobs inside its own transaction; workers in the API process claim due jobs,
// retry failures with backoff and recover jobs whose worker died.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type Task[T any] struct {
	kind string
}

func Define[T any](kind string) Task[T] {
	return Task[T]{kind: kind}
}

func (t Task[T]) Kind() string {
	return t.kind
}

type Options struct {
	Timeout     time.Duration
	MaxAttempts int
}

type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func Enqueue[T any](ctx context.Context, tx execer, task Task[T], args T) error {
	payload, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", task.kind, err)
	}

	if _, err := tx.Exec(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1, $2)`, task.kind, payload); err != nil {
		return fmt.Errorf("enqueue %s: %w", task.kind, err)
	}
	return nil
}

func Register[T any](r *Runner, task Task[T], handle func(context.Context, T) error, opts Options) error {
	if opts.Timeout <= 0 || opts.MaxAttempts <= 0 {
		return fmt.Errorf("register %s: timeout and max attempts must be positive", task.kind)
	}
	if _, ok := r.handlers[task.kind]; ok {
		return fmt.Errorf("register %s: task is already registered", task.kind)
	}

	r.handlers[task.kind] = registration{
		opts: opts,
		run: func(ctx context.Context, payload []byte) error {
			var args T
			if err := json.Unmarshal(payload, &args); err != nil {
				return Permanent(fmt.Errorf("decode payload: %w", err))
			}
			return handle(ctx, args)
		},
	}
	return nil
}

type permanentError struct {
	err error
}

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func Permanent(err error) error {
	return permanentError{err: err}
}

func isPermanent(err error) bool {
	var p permanentError
	return errors.As(err, &p)
}
