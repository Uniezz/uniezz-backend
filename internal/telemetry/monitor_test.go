package telemetry

import (
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

func TestIntervalSchedule(t *testing.T) {
	cases := []struct {
		name  string
		every time.Duration
		want  sentry.MonitorSchedule
	}{
		{"whole days", 30 * 24 * time.Hour, sentry.IntervalSchedule(30, sentry.MonitorScheduleUnitDay)},
		{"whole hours", 6 * time.Hour, sentry.IntervalSchedule(6, sentry.MonitorScheduleUnitHour)},
		{"minutes, rounded up", 90*time.Second + time.Minute, sentry.IntervalSchedule(3, sentry.MonitorScheduleUnitMinute)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := intervalSchedule(tc.every); got != tc.want {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}
