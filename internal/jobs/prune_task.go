package jobs

import (
	"context"
	"fmt"
	"time"
)

type pruneArgs struct{}

func (pruneArgs) Kind() string { return "jobs.prune.v1" }

func (r *Runner) prune(ctx context.Context, _ pruneArgs) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		DELETE FROM jobs
		WHERE (status = 'succeeded' AND finished_at < NOW() - INTERVAL '7 days')
		   OR (status = 'failed' AND finished_at < NOW() - INTERVAL '30 days')`)
	if err != nil {
		return fmt.Errorf("prune jobs: %w", err)
	}
	return nil
}

func (r *Runner) registerPrune() {
	r.handlers[pruneArgs{}.Kind()] = registration{
		opts: Options{Timeout: time.Minute, MaxAttempts: 3},
		run:  func(ctx context.Context, _ []byte) error { return r.prune(ctx, pruneArgs{}) },
	}
	r.schedules = append(r.schedules, schedule{kind: pruneArgs{}.Kind(), payload: []byte("{}"), every: 24 * time.Hour})
}
