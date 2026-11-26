package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/telegram"
)

const (
	alertCooldown  = 10 * time.Minute
	alertHourlyCap = 20
	alertQueueSize = 32
	alertMaxDetail = 300
)

var alerts atomic.Pointer[alerter]

type alerter struct {
	thread   telegram.Thread
	resource Resource
	now      func() time.Time
	queue    chan string
	done     chan struct{}

	mu           sync.Mutex
	closed       bool
	lastSent     map[string]time.Time
	suppressed   map[string]int
	windowStart  time.Time
	sentInWindow int
}

func newAlerter(thread telegram.Thread, now func() time.Time) *alerter {
	a := &alerter{
		thread:     thread,
		now:        now,
		queue:      make(chan string, alertQueueSize),
		done:       make(chan struct{}),
		lastSent:   map[string]time.Time{},
		suppressed: map[string]int{},
	}
	go a.run()
	return a
}

func (a *alerter) notify(key, text string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return
	}

	now := a.now()
	if last, ok := a.lastSent[key]; ok && now.Sub(last) < alertCooldown {
		a.suppressed[key]++
		return
	}
	if now.Sub(a.windowStart) >= time.Hour {
		a.windowStart, a.sentInWindow = now, 0
	}
	if a.sentInWindow >= alertHourlyCap {
		a.suppressed[key]++
		return
	}
	if n := a.suppressed[key]; n > 0 {
		text += fmt.Sprintf("\n+%d more since the last alert", n)
	}

	select {
	case a.queue <- text:
		a.lastSent[key] = now
		a.sentInWindow++
		delete(a.suppressed, key)
	default:
		a.suppressed[key]++
	}
}

func (a *alerter) notifyError(ctx context.Context, operation string, err error) {
	if a == nil {
		return
	}
	route := routeOf(ctx)
	a.notify("error "+operation+" "+route, a.text(ctx, "🔴", operation, route, err.Error()))
}

func (a *alerter) notifyPanic(ctx context.Context, recovered any) {
	if a == nil {
		return
	}
	route := routeOf(ctx)
	a.notify("panic "+route, a.text(ctx, "💥", "panic", route, fmt.Sprint(recovered)))
}

func (a *alerter) text(ctx context.Context, icon, operation, route, detail string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s · %s", icon, a.resource.Env, operation)
	if route != "" {
		fmt.Fprintf(&b, "\nroute: %s", route)
	}
	fmt.Fprintf(&b, "\nerror: %s", truncate(Scrub(detail), alertMaxDetail))
	fmt.Fprintf(&b, "\nversion: %s", a.resource.Version)
	if id := RequestID(ctx); id != "" {
		fmt.Fprintf(&b, "\nrequest: %s", id)
	}
	return b.String()
}

func truncate(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

func (a *alerter) run() {
	defer close(a.done)
	for text := range a.queue {
		if err := a.send(text); err != nil {
			slog.LogAttrs(context.Background(), slog.LevelWarn, "send telegram alert", slog.Any("error", err))
		}
	}
}

func (a *alerter) send(text string) error {
	if err := a.thread.Send(context.Background(), text); err != nil {
		return fmt.Errorf("post alert: %w", err)
	}
	return nil
}

func (a *alerter) shutdown(ctx context.Context) error {
	a.mu.Lock()
	if !a.closed {
		a.closed = true
		close(a.queue)
	}
	a.mu.Unlock()

	select {
	case <-a.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("telegram alerts: %w", ctx.Err())
	}
}
