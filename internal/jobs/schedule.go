package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/telemetry"
)

type schedule struct {
	kind    string
	payload []byte
	every   time.Duration
}

func Schedule(r *Runner, args Args, every time.Duration) error {
	kind := args.Kind()
	if every < time.Minute {
		return fmt.Errorf("schedule %s: interval must be at least a minute", kind)
	}
	if _, ok := r.handlers[kind]; !ok {
		return fmt.Errorf("schedule %s: task is not registered", kind)
	}

	payload, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("encode %s payload: %w", kind, err)
	}
	r.schedules = append(r.schedules, schedule{kind: kind, payload: payload, every: every})
	return nil
}

func (r *Runner) runSchedules(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		r.enqueueDue(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) enqueueDue(ctx context.Context, now time.Time) {
	for _, s := range r.schedules {
		slot := now.Truncate(s.every)
		key := s.kind + "@" + slot.UTC().Format(time.RFC3339)

		tag, err := r.db.Conn(ctx).Exec(ctx, `
			INSERT INTO jobs (kind, payload, unique_key, available_at)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (unique_key) WHERE unique_key IS NOT NULL DO NOTHING`,
			s.kind, s.payload, key, slot)
		if err != nil {
			if ctx.Err() == nil {
				telemetry.ReportError(ctx, "enqueue scheduled job", err, slog.String("job_kind", s.kind))
			}
			continue
		}
		if tag.RowsAffected() == 1 {
			r.Wake()
		}
	}
}
