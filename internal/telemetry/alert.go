package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	telegramAPI      = "https://api.telegram.org"
	alertCooldown    = 10 * time.Minute
	alertHourlyCap   = 20
	alertQueueSize   = 32
	alertSendTimeout = 5 * time.Second
	alertMaxDetail   = 300
)

var alerts atomic.Pointer[alerter]

type alerter struct {
	baseURL  string
	token    string
	chatID   string
	resource Resource
	client   *http.Client
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

type sendMessageRequest struct {
	ChatID             string             `json:"chat_id"`
	Text               string             `json:"text"`
	LinkPreviewOptions linkPreviewOptions `json:"link_preview_options"`
}

type linkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

func newAlerter(baseURL, token, chatID string, now func() time.Time) *alerter {
	a := &alerter{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		chatID:     chatID,
		client:     &http.Client{Timeout: alertSendTimeout},
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
	body, err := json.Marshal(sendMessageRequest{
		ChatID:             a.chatID,
		Text:               text,
		LinkPreviewOptions: linkPreviewOptions{IsDisabled: true},
	})
	if err != nil {
		return fmt.Errorf("encode alert: %w", err)
	}

	resp, err := a.client.Post(a.baseURL+"/bot"+a.token+"/sendMessage", "application/json", bytes.NewReader(body))
	if err != nil {
		// The *url.Error repeats the request URL, which holds the bot token.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("post alert: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("post alert: telegram answered %d", resp.StatusCode)
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
