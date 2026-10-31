package jobs

import (
	"context"
	"testing"
	"time"
)

func TestSchedule(t *testing.T) {
	t.Run("rejects unregistered tasks and intervals under a minute", func(t *testing.T) {
		r := NewRunner(nil, 1, 0)
		if err := Schedule(r, testArgs{}, time.Hour); err == nil {
			t.Error("expected an error for a task without a handler")
		}
		register(t, r, quick, func(context.Context, testArgs) error { return nil })
		if err := Schedule(r, testArgs{}, time.Second); err == nil {
			t.Error("expected an error for an interval under a minute")
		}
		if err := Schedule(r, testArgs{}, time.Hour); err != nil {
			t.Errorf("Schedule failed: %v", err)
		}
	})

	pool := setupTestDB(t)
	ctx := context.Background()

	countTestJobs := func(t *testing.T) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs WHERE kind = $1", testArgs{}.Kind()).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("enqueues one job per slot however often it runs", func(t *testing.T) {
		cleanJobs(t, pool)
		r := NewRunner(pool, 1, 0)
		r.schedules = nil
		register(t, r, quick, func(context.Context, testArgs) error { return nil })
		if err := Schedule(r, testArgs{N: 3}, time.Hour); err != nil {
			t.Fatal(err)
		}

		now := time.Date(2026, 10, 31, 12, 15, 0, 0, time.UTC)
		r.enqueueDue(ctx, now)
		r.enqueueDue(ctx, now.Add(30*time.Minute))
		other := NewRunner(pool, 1, 0)
		other.schedules = r.schedules
		other.enqueueDue(ctx, now)
		if n := countTestJobs(t); n != 1 {
			t.Fatalf("expected one job for the 12:00 slot, got %d", n)
		}

		r.enqueueDue(ctx, now.Add(time.Hour))
		if n := countTestJobs(t); n != 2 {
			t.Fatalf("expected a second job for the 13:00 slot, got %d", n)
		}

		var payload string
		var availableAt time.Time
		err := pool.QueryRow(ctx, "SELECT payload::text, available_at FROM jobs WHERE unique_key = $1", "test.job@2026-10-31T12:00:00Z").Scan(&payload, &availableAt)
		if err != nil {
			t.Fatalf("expected the 12:00 job: %v", err)
		}
		if payload != `{"n": 3}` || !availableAt.Equal(now.Truncate(time.Hour)) {
			t.Errorf("job = %s at %v, want {\"n\": 3} at 12:00", payload, availableAt)
		}
	})

	t.Run("wakes the workers only when it enqueued something", func(t *testing.T) {
		cleanJobs(t, pool)
		r := NewRunner(pool, 1, 0)
		r.schedules = nil
		register(t, r, quick, func(context.Context, testArgs) error { return nil })
		if err := Schedule(r, testArgs{}, time.Hour); err != nil {
			t.Fatal(err)
		}

		now := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)
		r.enqueueDue(ctx, now)
		select {
		case <-r.wake:
		default:
			t.Fatal("expected a wakeup after enqueueing")
		}

		r.enqueueDue(ctx, now)
		select {
		case <-r.wake:
			t.Error("expected no wakeup when the slot was already taken")
		default:
		}
	})
}

func TestPrune(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	cleanJobs(t, pool)

	insert := func(status string, finishedAgo time.Duration) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO jobs (kind, payload, status, finished_at)
			VALUES ($1, '{}', $2, NOW() - $3::interval)`, testArgs{}.Kind(), status, finishedAgo)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert("succeeded", 8*24*time.Hour)
	insert("succeeded", 6*24*time.Hour)
	insert("failed", 8*24*time.Hour)
	insert("failed", 31*24*time.Hour)

	if err := NewRunner(pool, 1, 0).prune(ctx, pruneArgs{}); err != nil {
		t.Fatalf("prune failed: %v", err)
	}

	rows, err := pool.Query(ctx, "SELECT status, NOW() - finished_at > INTERVAL '7 days' FROM jobs WHERE kind = $1 ORDER BY status, finished_at DESC", testArgs{}.Kind())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var left []string
	for rows.Next() {
		var status string
		var old bool
		if err := rows.Scan(&status, &old); err != nil {
			t.Fatal(err)
		}
		if old {
			status += " (old)"
		}
		left = append(left, status)
	}
	want := []string{"failed (old)", "succeeded"}
	if len(left) != len(want) || left[0] != want[0] || left[1] != want[1] {
		t.Errorf("jobs left = %v, want %v", left, want)
	}
}
