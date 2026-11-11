package jobs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func captureSentry(t *testing.T) *sentry.MockTransport {
	t.Helper()
	transport := &sentry.MockTransport{}
	if err := sentry.Init(sentry.ClientOptions{Dsn: "https://public@o1.ingest.sentry.io/1", Transport: transport}); err != nil {
		t.Fatalf("init sentry: %v", err)
	}
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })
	return transport
}

func checkIns(transport *sentry.MockTransport) []*sentry.Event {
	var out []*sentry.Event
	for _, e := range transport.Events() {
		if e.CheckIn != nil {
			out = append(out, e)
		}
	}
	return out
}

func TestScheduleNames(t *testing.T) {
	r := NewRunner(nil, 1, 0)
	register(t, r, quick, func(context.Context, testArgs) error { return nil })
	cases := []struct {
		name    string
		ok      bool
		comment string
	}{
		{"test-job", true, "a lowercase slug"},
		{"test-job", false, "a name used twice"},
		{"Test_Job", false, "uppercase and underscores"},
		{"test.job", false, "a dot"},
		{"", false, "an empty name"},
		{"a123456789-123456789-123456789-123456789-1234567890", false, "over 50 characters"},
	}
	for _, tc := range cases {
		t.Run(tc.comment, func(t *testing.T) {
			err := Schedule(r, tc.name, testArgs{}, time.Hour)
			if (err == nil) != tc.ok {
				t.Errorf("Schedule(%q) error = %v, want ok %v", tc.name, err, tc.ok)
			}
		})
	}
}

func TestScheduledJobCheckIns(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()

	t.Run("a scheduled run sends in progress, then ok with the monitor config", func(t *testing.T) {
		cleanJobs(t, pool)
		transport := captureSentry(t)
		r := NewRunner(pool, 1, 0)
		r.schedules = nil
		register(t, r, quick, func(context.Context, testArgs) error { return nil })
		if err := Schedule(r, "test-job", testArgs{}, 24*time.Hour); err != nil {
			t.Fatal(err)
		}
		r.enqueueDue(ctx, time.Now())
		if ran, err := r.runNext(ctx); !ran || err != nil {
			t.Fatalf("runNext = %v, %v", ran, err)
		}

		events := checkIns(transport)
		if len(events) != 2 {
			t.Fatalf("check-ins = %d, want 2", len(events))
		}
		start, end := events[0], events[1]
		if start.CheckIn.MonitorSlug != "test-job" || start.CheckIn.Status != sentry.CheckInStatusInProgress {
			t.Errorf("start = %#v", start.CheckIn)
		}
		if start.MonitorConfig == nil || start.MonitorConfig.FailureIssueThreshold != int64(quick.MaxAttempts) {
			t.Errorf("monitor config = %#v", start.MonitorConfig)
		}
		if end.CheckIn.ID != start.CheckIn.ID || end.CheckIn.Status != sentry.CheckInStatusOK {
			t.Errorf("end = %#v", end.CheckIn)
		}
	})

	t.Run("a failed scheduled run sends error", func(t *testing.T) {
		cleanJobs(t, pool)
		transport := captureSentry(t)
		r := NewRunner(pool, 1, 0)
		r.schedules = nil
		register(t, r, quick, func(context.Context, testArgs) error { return errors.New("boom") })
		if err := Schedule(r, "test-job", testArgs{}, time.Hour); err != nil {
			t.Fatal(err)
		}
		r.enqueueDue(ctx, time.Now())
		if _, err := r.runNext(ctx); err != nil {
			t.Fatal(err)
		}
		events := checkIns(transport)
		if len(events) != 2 || events[1].CheckIn.Status != sentry.CheckInStatusError {
			t.Fatalf("check-ins = %#v", events)
		}
	})

	t.Run("an enqueued job sends no check-in", func(t *testing.T) {
		cleanJobs(t, pool)
		transport := captureSentry(t)
		r := NewRunner(pool, 1, 0)
		r.schedules = nil
		register(t, r, quick, func(context.Context, testArgs) error { return nil })
		enqueue(t, pool, testArgs{})
		if _, err := r.runNext(ctx); err != nil {
			t.Fatal(err)
		}
		if events := checkIns(transport); len(events) != 0 {
			t.Fatalf("check-ins = %d, want 0", len(events))
		}
	})
}
