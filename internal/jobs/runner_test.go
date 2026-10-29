package jobs

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type testArgs struct {
	N int `json:"n"`
}

func newTestTask(t *testing.T, pool *pgxpool.Pool) Task[testArgs] {
	t.Helper()

	task := Define[testArgs]("test." + uuid.NewString())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM jobs WHERE kind = $1", task.Kind())
	})
	return task
}

func enqueue(t *testing.T, pool *pgxpool.Pool, task Task[testArgs], args testArgs) {
	t.Helper()
	if err := Enqueue(context.Background(), pool, task, args); err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
}

func register(t *testing.T, r *Runner, task Task[testArgs], opts Options, handle func(context.Context, testArgs) error) {
	t.Helper()
	if err := Register(r, task, handle, opts); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
}

type jobRow struct {
	Status      string
	Attempts    int
	LastError   *string
	AvailableAt time.Time
	Token       *uuid.UUID
}

func onlyJob(t *testing.T, pool *pgxpool.Pool, kind string) jobRow {
	t.Helper()

	var j jobRow
	err := pool.QueryRow(context.Background(),
		"SELECT status, attempts, last_error, available_at, execution_token FROM jobs WHERE kind = $1", kind).
		Scan(&j.Status, &j.Attempts, &j.LastError, &j.AvailableAt, &j.Token)
	if err != nil {
		t.Fatalf("failed to read job: %v", err)
	}
	return j
}

func runNext(t *testing.T, r *Runner) bool {
	t.Helper()
	ran, err := r.runNext(context.Background())
	if err != nil {
		t.Fatalf("runNext failed: %v", err)
	}
	return ran
}

var quick = Options{Timeout: time.Second, MaxAttempts: 3}

func TestRunNext(t *testing.T) {
	pool := setupTestDB(t)

	t.Run("a successful job is marked succeeded and gets its typed payload", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		var got testArgs
		register(t, r, task, quick, func(_ context.Context, a testArgs) error { got = a; return nil })
		enqueue(t, pool, task, testArgs{N: 7})

		if !runNext(t, r) {
			t.Fatal("expected a job to run")
		}
		if got.N != 7 {
			t.Errorf("handler got %+v, want N=7", got)
		}
		if j := onlyJob(t, pool, task.Kind()); j.Status != "succeeded" || j.Attempts != 1 || j.Token != nil {
			t.Errorf("job = %+v, want succeeded after 1 attempt without a token", j)
		}
		if runNext(t, r) {
			t.Error("a succeeded job must not run again")
		}
	})

	t.Run("a temporary failure goes back to pending with a delay and the error", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		register(t, r, task, quick, func(context.Context, testArgs) error { return errors.New("storage down") })
		enqueue(t, pool, task, testArgs{})

		runNext(t, r)

		j := onlyJob(t, pool, task.Kind())
		if j.Status != "pending" || j.Attempts != 1 || j.LastError == nil || *j.LastError != "storage down" {
			t.Fatalf("job = %+v, want pending after 1 attempt with the error saved", j)
		}
		if time.Until(j.AvailableAt) < minBackoff/2-time.Second {
			t.Errorf("retry is due in %v, want at least %v", time.Until(j.AvailableAt), minBackoff/2)
		}
		if runNext(t, r) {
			t.Error("a delayed retry must not be claimed before it is due")
		}
	})

	t.Run("a permanent failure stops retrying", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		register(t, r, task, quick, func(context.Context, testArgs) error { return Permanent(errors.New("bad image")) })
		enqueue(t, pool, task, testArgs{})

		runNext(t, r)

		if j := onlyJob(t, pool, task.Kind()); j.Status != "failed" || j.Attempts != 1 {
			t.Errorf("job = %+v, want failed after 1 attempt", j)
		}
	})

	t.Run("the last allowed attempt fails the job", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		register(t, r, task, Options{Timeout: time.Second, MaxAttempts: 2}, func(context.Context, testArgs) error { return errors.New("down") })
		enqueue(t, pool, task, testArgs{})

		runNext(t, r)
		if _, err := pool.Exec(context.Background(), "UPDATE jobs SET available_at = NOW() WHERE kind = $1", task.Kind()); err != nil {
			t.Fatal(err)
		}
		runNext(t, r)

		if j := onlyJob(t, pool, task.Kind()); j.Status != "failed" || j.Attempts != 2 {
			t.Errorf("job = %+v, want failed after 2 attempts", j)
		}
	})

	t.Run("a malformed payload fails without calling the handler", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		called := false
		register(t, r, task, quick, func(context.Context, testArgs) error { called = true; return nil })
		if _, err := pool.Exec(context.Background(), `INSERT INTO jobs (kind, payload) VALUES ($1, '{"n":"x"}')`, task.Kind()); err != nil {
			t.Fatal(err)
		}

		runNext(t, r)

		if called {
			t.Error("handler must not run for a payload that does not decode")
		}
		if j := onlyJob(t, pool, task.Kind()); j.Status != "failed" {
			t.Errorf("job = %+v, want failed", j)
		}
	})

	t.Run("a handler that overruns its timeout gets a cancelled context", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		register(t, r, task, Options{Timeout: 50 * time.Millisecond, MaxAttempts: 3}, func(ctx context.Context, _ testArgs) error {
			<-ctx.Done()
			return ctx.Err()
		})
		enqueue(t, pool, task, testArgs{})

		runNext(t, r)

		if j := onlyJob(t, pool, task.Kind()); j.Status != "pending" || j.LastError == nil {
			t.Errorf("job = %+v, want pending for a retry after the timeout", j)
		}
	})

	t.Run("a panicking handler fails the job instead of the process", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		register(t, r, task, quick, func(context.Context, testArgs) error { panic("nil map") })
		enqueue(t, pool, task, testArgs{})

		runNext(t, r)

		if j := onlyJob(t, pool, task.Kind()); j.Status != "failed" || j.LastError == nil {
			t.Errorf("job = %+v, want failed with the panic saved", j)
		}
	})

	t.Run("jobs of other kinds are left alone", func(t *testing.T) {
		r := NewRunner(pool, 1)
		mine := newTestTask(t, pool)
		other := newTestTask(t, pool)
		register(t, r, mine, quick, func(context.Context, testArgs) error { return nil })
		enqueue(t, pool, other, testArgs{})

		if runNext(t, r) {
			t.Error("runner must not claim a job it has no handler for")
		}
		if j := onlyJob(t, pool, other.Kind()); j.Status != "pending" {
			t.Errorf("other job = %+v, want pending", j)
		}
	})
}

func TestRecovery(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	expireLease := func(t *testing.T, kind string) {
		t.Helper()
		if _, err := pool.Exec(ctx, "UPDATE jobs SET locked_until = NOW() - interval '1 second' WHERE kind = $1", kind); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a job whose worker died is claimed again after its lease expires", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		register(t, r, task, quick, func(context.Context, testArgs) error { return nil })
		enqueue(t, pool, task, testArgs{})

		if _, err := r.claim(ctx); err != nil {
			t.Fatalf("claim failed: %v", err)
		}
		if runNext(t, r) {
			t.Fatal("a job with a live lease must not be claimed twice")
		}

		expireLease(t, task.Kind())
		if !runNext(t, r) {
			t.Fatal("expected the interrupted job to be recovered")
		}
		if j := onlyJob(t, pool, task.Kind()); j.Status != "succeeded" || j.Attempts != 2 {
			t.Errorf("job = %+v, want succeeded on attempt 2", j)
		}
	})

	t.Run("an interrupted job with no attempts left fails without running", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		called := false
		register(t, r, task, Options{Timeout: time.Second, MaxAttempts: 1}, func(context.Context, testArgs) error { called = true; return nil })
		enqueue(t, pool, task, testArgs{})

		if _, err := r.claim(ctx); err != nil {
			t.Fatalf("claim failed: %v", err)
		}
		expireLease(t, task.Kind())
		runNext(t, r)

		if called {
			t.Error("handler must not run past the attempt limit")
		}
		if j := onlyJob(t, pool, task.Kind()); j.Status != "failed" {
			t.Errorf("job = %+v, want failed", j)
		}
	})

	t.Run("a stale attempt cannot overwrite the state of a newer one", func(t *testing.T) {
		r := NewRunner(pool, 1)
		task := newTestTask(t, pool)
		register(t, r, task, quick, func(context.Context, testArgs) error { return nil })
		enqueue(t, pool, task, testArgs{})

		stale, err := r.claim(ctx)
		if err != nil {
			t.Fatalf("claim failed: %v", err)
		}
		expireLease(t, task.Kind())
		current, err := r.claim(ctx)
		if err != nil {
			t.Fatalf("second claim failed: %v", err)
		}

		if err := r.succeed(stale); err != nil {
			t.Fatalf("succeed failed: %v", err)
		}

		j := onlyJob(t, pool, task.Kind())
		if j.Status != "running" || j.Token == nil || *j.Token != current.token {
			t.Errorf("job = %+v, want still running under the newer token", j)
		}
	})
}

func TestEnqueueRollsBackWithTheTransaction(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	task := newTestTask(t, pool)
	boom := errors.New("account creation failed")

	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if err := Enqueue(ctx, tx, task, testArgs{}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected the transaction error, got %v", err)
	}

	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind = $1", task.Kind()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expected no job after rollback, got %d", n)
	}
}

func runInBackground(t *testing.T, r *Runner) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func countSucceeded(t *testing.T, pool *pgxpool.Pool, kind string) int {
	t.Helper()

	var n int
	err := pool.QueryRow(context.Background(), "SELECT count(*) FROM jobs WHERE kind = $1 AND status = 'succeeded'", kind).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRun(t *testing.T) {
	pool := setupTestDB(t)

	t.Run("jobs committed before start are picked up without a wakeup", func(t *testing.T) {
		r := NewRunner(pool, 1)
		r.pollInterval = time.Hour
		task := newTestTask(t, pool)
		register(t, r, task, quick, func(context.Context, testArgs) error { return nil })
		enqueue(t, pool, task, testArgs{})

		runInBackground(t, r)

		waitFor(t, "the job to succeed", func() bool { return countSucceeded(t, pool, task.Kind()) == 1 })
	})

	t.Run("one wakeup drains every due job, and a delayed retry does not block them", func(t *testing.T) {
		r := NewRunner(pool, 1)
		r.pollInterval = time.Hour
		task := newTestTask(t, pool)
		register(t, r, task, quick, func(_ context.Context, a testArgs) error {
			if a.N == 0 {
				return errors.New("always fails")
			}
			return nil
		})

		runInBackground(t, r)
		time.Sleep(50 * time.Millisecond)

		for i := range 10 {
			enqueue(t, pool, task, testArgs{N: i})
		}
		r.Wake()

		waitFor(t, "nine jobs to succeed", func() bool { return countSucceeded(t, pool, task.Kind()) == 9 })
	})

	t.Run("several workers never run the same job twice", func(t *testing.T) {
		r := NewRunner(pool, 3)
		r.pollInterval = 20 * time.Millisecond
		task := newTestTask(t, pool)

		var mu sync.Mutex
		runs := map[int]int{}
		register(t, r, task, quick, func(_ context.Context, a testArgs) error {
			mu.Lock()
			runs[a.N]++
			mu.Unlock()
			time.Sleep(5 * time.Millisecond)
			return nil
		})
		for i := range 30 {
			enqueue(t, pool, task, testArgs{N: i})
		}

		runInBackground(t, r)
		waitFor(t, "all jobs to succeed", func() bool { return countSucceeded(t, pool, task.Kind()) == 30 })

		mu.Lock()
		defer mu.Unlock()
		for n, count := range runs {
			if count != 1 {
				t.Errorf("job %d ran %d times", n, count)
			}
		}
	})
}

func TestRegister(t *testing.T) {
	r := NewRunner(nil, 1)
	task := Define[testArgs]("test.register")
	noop := func(context.Context, testArgs) error { return nil }

	if err := Register(r, task, noop, Options{}); err == nil {
		t.Error("expected an error for zero timeout and attempts")
	}
	if err := Register(r, task, noop, quick); err != nil {
		t.Fatalf("Register failed: %v", err)
	}
	if err := Register(r, task, noop, quick); err == nil {
		t.Error("expected an error for a duplicate registration")
	}
}

func TestBackoff(t *testing.T) {
	for attempt := 1; attempt <= 20; attempt++ {
		ceiling := min(minBackoff<<min(attempt-1, 10), maxBackoff)
		for range 50 {
			if d := backoff(attempt); d < ceiling/2 || d > ceiling {
				t.Fatalf("backoff(%d) = %v, want between %v and %v", attempt, d, ceiling/2, ceiling)
			}
		}
	}
}
