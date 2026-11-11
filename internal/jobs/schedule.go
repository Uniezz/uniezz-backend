package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/telemetry"
)

const slotSeparator = "@"

var scheduleNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,49}$`)

type schedule struct {
	name    string
	kind    string
	payload []byte
	every   time.Duration
}

// Schedule runs the task every interval. The name is unique per schedule: it
// keys one job per slot and names the Sentry Crons monitor, so it is a slug of
// at most 50 characters.
func Schedule(r *Runner, name string, args Args, every time.Duration) error {
	kind := args.Kind()
	if !scheduleNamePattern.MatchString(name) {
		return fmt.Errorf("schedule %s: name %q must be a lowercase slug of at most 50 characters", kind, name)
	}
	if _, ok := r.scheduleNamed(name); ok {
		return fmt.Errorf("schedule %s: name %q is already used", kind, name)
	}
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
	r.schedules = append(r.schedules, schedule{name: name, kind: kind, payload: payload, every: every})
	return nil
}

func (r *Runner) scheduleNamed(name string) (schedule, bool) {
	for _, s := range r.schedules {
		if s.name == name {
			return s, true
		}
	}
	return schedule{}, false
}

func (r *Runner) scheduleOf(uniqueKey *string) (schedule, bool) {
	if uniqueKey == nil {
		return schedule{}, false
	}
	name, _, ok := strings.Cut(*uniqueKey, slotSeparator)
	if !ok {
		return schedule{}, false
	}
	return r.scheduleNamed(name)
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
		key := s.name + slotSeparator + slot.UTC().Format(time.RFC3339)

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
