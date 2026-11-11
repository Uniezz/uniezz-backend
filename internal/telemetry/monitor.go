package telemetry

import (
	"context"
	"math"
	"time"

	"github.com/getsentry/sentry-go"
)

const checkInMargin = 15 * time.Minute

type MonitorSchedule struct {
	Every       time.Duration
	MaxRuntime  time.Duration
	MaxAttempts int
}

type CheckIn struct {
	hub     *sentry.Hub
	slug    string
	id      *sentry.EventID
	started time.Time
}

// StartCheckIn tells Sentry Crons that a scheduled run of the monitor began.
// The config it sends creates or updates the monitor, so no monitor is set up
// by hand. A monitor opens an issue only after every attempt of a run failed.
func StartCheckIn(ctx context.Context, slug string, s MonitorSchedule) CheckIn {
	hub := hubFor(ctx)
	config := &sentry.MonitorConfig{
		Schedule:              intervalSchedule(s.Every),
		CheckInMargin:         ceilMinutes(checkInMargin),
		MaxRuntime:            ceilMinutes(s.MaxRuntime),
		Timezone:              "UTC",
		FailureIssueThreshold: int64(max(s.MaxAttempts, 1)),
		RecoveryThreshold:     1,
	}
	id := hub.CaptureCheckIn(&sentry.CheckIn{MonitorSlug: slug, Status: sentry.CheckInStatusInProgress}, config)
	return CheckIn{hub: hub, slug: slug, id: id, started: time.Now()}
}

func (c CheckIn) Finish(err error) {
	if c.id == nil {
		return
	}
	status := sentry.CheckInStatusOK
	if err != nil {
		status = sentry.CheckInStatusError
	}
	c.hub.CaptureCheckIn(&sentry.CheckIn{
		ID:          *c.id,
		MonitorSlug: c.slug,
		Status:      status,
		Duration:    time.Since(c.started),
	}, nil)
}

func intervalSchedule(every time.Duration) sentry.MonitorSchedule {
	switch {
	case every%(24*time.Hour) == 0:
		return sentry.IntervalSchedule(int64(every/(24*time.Hour)), sentry.MonitorScheduleUnitDay)
	case every%time.Hour == 0:
		return sentry.IntervalSchedule(int64(every/time.Hour), sentry.MonitorScheduleUnitHour)
	default:
		return sentry.IntervalSchedule(ceilMinutes(every), sentry.MonitorScheduleUnitMinute)
	}
}

func ceilMinutes(d time.Duration) int64 {
	return int64(math.Ceil(d.Minutes()))
}
