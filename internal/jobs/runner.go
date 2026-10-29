package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultPollInterval = 30 * time.Second
	leaseMargin         = 30 * time.Second
	minBackoff          = 10 * time.Second
	maxBackoff          = 10 * time.Minute
	finishTimeout       = 5 * time.Second
)

type registration struct {
	opts Options
	run  func(ctx context.Context, payload []byte) error
}

type Runner struct {
	db           *pgxpool.Pool
	workers      int
	pollInterval time.Duration
	handlers     map[string]registration
	wake         chan struct{}
}

func NewRunner(db *pgxpool.Pool, workers int) *Runner {
	return &Runner{
		db:           db,
		workers:      max(workers, 1),
		pollInterval: defaultPollInterval,
		handlers:     make(map[string]registration),
		wake:         make(chan struct{}, 1),
	}
}

func (r *Runner) Wake() {
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *Runner) Run(ctx context.Context) {
	r.reportUnknownKinds(ctx)

	var wg sync.WaitGroup
	for range r.workers {
		wg.Go(func() { r.work(ctx) })
	}
	wg.Wait()
}

func (r *Runner) work(ctx context.Context) {
	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		r.drain(ctx)
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-ticker.C:
		}
	}
}

func (r *Runner) drain(ctx context.Context) {
	for ctx.Err() == nil {
		ran, err := r.runNext(ctx)
		if err != nil {
			if ctx.Err() == nil {
				telemetry.ReportError(ctx, "run background job", err)
			}
			return
		}
		if !ran {
			return
		}
	}
}

type claimedJob struct {
	id       uuid.UUID
	kind     string
	payload  []byte
	attempts int
	token    uuid.UUID
}

func (r *Runner) runNext(ctx context.Context) (bool, error) {
	job, err := r.claim(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	r.Wake()

	attrs := []slog.Attr{slog.String("job_kind", job.kind), slog.String("job_id", job.id.String()), slog.Int("attempt", job.attempts)}

	reg := r.handlers[job.kind]
	if job.attempts > reg.opts.MaxAttempts {
		return true, r.fail(job, errors.New("attempts exhausted after an interrupted run"), attrs)
	}

	err = runHandler(ctx, reg, job.payload)

	switch {
	case err == nil:
		slog.LogAttrs(ctx, slog.LevelInfo, "background job succeeded", attrs...)
		return true, r.succeed(job)
	case isPermanent(err) || job.attempts >= reg.opts.MaxAttempts:
		return true, r.fail(job, err, attrs)
	default:
		delay := backoff(job.attempts)
		slog.LogAttrs(ctx, slog.LevelWarn, "background job failed, will retry",
			append(attrs, slog.Any("error", err), slog.Duration("retry_in", delay))...)
		return true, r.retry(job, err, delay)
	}
}

func runHandler(ctx context.Context, reg registration, payload []byte) (err error) {
	ctx, cancel := context.WithTimeout(ctx, reg.opts.Timeout)
	defer cancel()
	defer func() {
		if p := recover(); p != nil {
			err = Permanent(fmt.Errorf("handler panicked: %v", p))
		}
	}()
	return reg.run(ctx, payload)
}

func (r *Runner) kinds() []string {
	kinds := make([]string, 0, len(r.handlers))
	for kind := range r.handlers {
		kinds = append(kinds, kind)
	}
	return kinds
}

func (r *Runner) reportUnknownKinds(ctx context.Context) {
	rows, err := r.db.Query(ctx, `
		SELECT kind, count(*) FROM jobs
		WHERE status IN ('pending', 'running') AND kind <> ALL($1)
		GROUP BY kind`, r.kinds())
	if err != nil {
		telemetry.ReportError(ctx, "check unknown job kinds", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var kind string
		var count int
		if err := rows.Scan(&kind, &count); err != nil {
			telemetry.ReportError(ctx, "check unknown job kinds", err)
			return
		}
		telemetry.ReportError(ctx, "jobs without a registered handler", errors.New("unknown job kind"),
			slog.String("job_kind", kind), slog.Int("count", count))
	}
}

func (r *Runner) lease() time.Duration {
	var longest time.Duration
	for _, reg := range r.handlers {
		longest = max(longest, reg.opts.Timeout)
	}
	return longest + leaseMargin
}

func (r *Runner) claim(ctx context.Context) (claimedJob, error) {
	const query = `
		UPDATE jobs
		SET status = 'running',
			attempts = attempts + 1,
			locked_until = NOW() + $1::interval,
			execution_token = gen_random_uuid()
		WHERE id = (
			SELECT id FROM jobs
			WHERE kind = ANY($2)
			  AND ((status = 'pending' AND available_at <= NOW())
			    OR (status = 'running' AND locked_until < NOW()))
			ORDER BY available_at
			LIMIT 1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, kind, payload, attempts, execution_token`

	var job claimedJob
	err := r.db.QueryRow(ctx, query, r.lease(), r.kinds()).Scan(&job.id, &job.kind, &job.payload, &job.attempts, &job.token)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return claimedJob{}, err
		}
		return claimedJob{}, fmt.Errorf("claim job: %w", err)
	}
	return job, nil
}

func (r *Runner) succeed(job claimedJob) error {
	return r.finish(job, `
		UPDATE jobs
		SET status = 'succeeded', locked_until = NULL, execution_token = NULL, last_error = NULL, finished_at = NOW()
		WHERE id = $1 AND execution_token = $2`)
}

func (r *Runner) retry(job claimedJob, cause error, delay time.Duration) error {
	return r.finish(job, `
		UPDATE jobs
		SET status = 'pending', locked_until = NULL, execution_token = NULL,
			available_at = NOW() + $3::interval, last_error = $4
		WHERE id = $1 AND execution_token = $2`, delay, cause.Error())
}

func (r *Runner) fail(job claimedJob, cause error, attrs []slog.Attr) error {
	telemetry.ReportError(context.Background(), "background job failed", cause, attrs...)

	return r.finish(job, `
		UPDATE jobs
		SET status = 'failed', locked_until = NULL, execution_token = NULL, last_error = $3, finished_at = NOW()
		WHERE id = $1 AND execution_token = $2`, cause.Error())
}

func (r *Runner) finish(job claimedJob, query string, args ...any) error {
	ctx, cancel := context.WithTimeout(context.Background(), finishTimeout)
	defer cancel()

	if _, err := r.db.Exec(ctx, query, append([]any{job.id, job.token}, args...)...); err != nil {
		return fmt.Errorf("record result of job %s: %w", job.id, err)
	}
	return nil
}

func backoff(attempt int) time.Duration {
	d := minBackoff << min(attempt-1, 10)
	d = min(d, maxBackoff)
	return d/2 + rand.N(d/2+1)
}
