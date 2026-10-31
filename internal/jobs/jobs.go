// Package jobs runs durable background tasks stored in PostgreSQL. A feature
// module defines an args type with a Kind, registers its handler at startup
// and enqueues jobs inside its own transaction; workers in the API process claim due jobs,
// retry failures with backoff and recover jobs whose worker died.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Args interface {
	Kind() string
}

type Options struct {
	Timeout     time.Duration
	MaxAttempts int
}

func (r *Runner) Enqueue(ctx context.Context, args Args) error {
	kind := args.Kind()
	payload, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", kind, err)
	}

	if _, err := r.db.Conn(ctx).Exec(ctx, `INSERT INTO jobs (kind, payload) VALUES ($1, $2)`, kind, payload); err != nil {
		return fmt.Errorf("enqueue %s: %w", kind, err)
	}
	return nil
}

func Register[T Args](r *Runner, handle func(context.Context, T) error, opts Options) error {
	var zero T
	kind := zero.Kind()
	if opts.Timeout <= 0 || opts.MaxAttempts <= 0 {
		return fmt.Errorf("register %s: timeout and max attempts must be positive", kind)
	}
	if _, ok := r.handlers[kind]; ok {
		return fmt.Errorf("register %s: task is already registered", kind)
	}

	r.handlers[kind] = registration{
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
