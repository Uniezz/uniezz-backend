package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/telegram"
)

const (
	testBotToken = "123456:bot-secret-token"
	testChatID   = "-1001234567890"
	testThreadID = 825
)

type sendMessageRequest struct {
	ChatID             string `json:"chat_id"`
	MessageThreadID    int    `json:"message_thread_id"`
	Text               string `json:"text"`
	LinkPreviewOptions struct {
		IsDisabled bool `json:"is_disabled"`
	} `json:"link_preview_options"`
}

func testThread(url string) telegram.Thread {
	return telegram.New(url, testBotToken, testChatID).Thread(testThreadID)
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type fakeTelegram struct {
	t      *testing.T
	server *httptest.Server
	status int

	mu       sync.Mutex
	messages []sendMessageRequest
}

func newFakeTelegram(t *testing.T) *fakeTelegram {
	t.Helper()
	f := &fakeTelegram{t: t, status: http.StatusOK}
	f.server = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeTelegram) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/bot"+testBotToken+"/sendMessage" {
		f.t.Errorf("request = %s %s, want POST /bot<token>/sendMessage", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusNotFound)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		f.t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var msg sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		f.t.Errorf("decode body: %v", err)
	}
	f.mu.Lock()
	f.messages = append(f.messages, msg)
	f.mu.Unlock()
	w.WriteHeader(f.status)
}

func (f *fakeTelegram) texts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	texts := make([]string, 0, len(f.messages))
	for _, m := range f.messages {
		texts = append(texts, m.Text)
	}
	return texts
}

func newTestAlerter(t *testing.T, f *fakeTelegram, clock *fakeClock) *alerter {
	t.Helper()
	a := newAlerter(testThread(f.server.URL), clock.Now)
	t.Cleanup(func() { _ = a.shutdown(context.Background()) })
	return a
}

func drain(t *testing.T, a *alerter) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestAlerter(t *testing.T) {
	t.Run("an alert is posted to the alerts topic with link previews off", func(t *testing.T) {
		f := newFakeTelegram(t)
		a := newTestAlerter(t, f, newFakeClock())

		a.notify("k", "something broke")
		drain(t, a)

		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.messages) != 1 {
			t.Fatalf("got %d messages, want 1", len(f.messages))
		}
		m := f.messages[0]
		if m.ChatID != testChatID || m.MessageThreadID != testThreadID || m.Text != "something broke" || !m.LinkPreviewOptions.IsDisabled {
			t.Errorf("message = %+v", m)
		}
	})

	t.Run("the same key within the cooldown is sent once", func(t *testing.T) {
		f := newFakeTelegram(t)
		clock := newFakeClock()
		a := newTestAlerter(t, f, clock)

		a.notify("k", "first")
		clock.Advance(alertCooldown - time.Second)
		a.notify("k", "second")
		drain(t, a)

		if got := f.texts(); len(got) != 1 || got[0] != "first" {
			t.Errorf("texts = %q, want [first]", got)
		}
	})

	t.Run("after the cooldown the key is sent again with the number it held back", func(t *testing.T) {
		f := newFakeTelegram(t)
		clock := newFakeClock()
		a := newTestAlerter(t, f, clock)

		a.notify("k", "first")
		a.notify("k", "held back")
		a.notify("k", "held back")
		clock.Advance(alertCooldown)
		a.notify("k", "again")
		drain(t, a)

		got := f.texts()
		if len(got) != 2 {
			t.Fatalf("texts = %q, want 2 messages", got)
		}
		if want := "again\n+2 more since the last alert"; got[1] != want {
			t.Errorf("second text = %q, want %q", got[1], want)
		}
	})

	t.Run("different keys are not deduplicated against each other", func(t *testing.T) {
		f := newFakeTelegram(t)
		a := newTestAlerter(t, f, newFakeClock())

		a.notify("a", "one")
		a.notify("b", "two")
		drain(t, a)

		if got := f.texts(); len(got) != 2 {
			t.Errorf("texts = %q, want 2 messages", got)
		}
	})

	t.Run("no more than the hourly cap is sent, and the window resets after an hour", func(t *testing.T) {
		f := newFakeTelegram(t)
		clock := newFakeClock()
		a := newTestAlerter(t, f, clock)

		for i := range alertHourlyCap + 5 {
			a.notify(fmt.Sprintf("key-%d", i), fmt.Sprintf("alert %d", i))
		}
		clock.Advance(time.Hour)
		a.notify(fmt.Sprintf("key-%d", alertHourlyCap), "after the window")
		drain(t, a)

		got := f.texts()
		if len(got) != alertHourlyCap+1 {
			t.Fatalf("got %d messages, want %d", len(got), alertHourlyCap+1)
		}
		if want := "after the window\n+1 more since the last alert"; got[alertHourlyCap] != want {
			t.Errorf("last text = %q, want %q", got[alertHourlyCap], want)
		}
	})

	t.Run("a full queue drops alerts instead of blocking the caller", func(t *testing.T) {
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-release
		}))
		t.Cleanup(server.Close)
		t.Cleanup(func() { close(release) })
		clock := newFakeClock()
		a := newAlerter(testThread(server.URL), clock.Now)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for i := range alertQueueSize + 5 {
				if i%alertHourlyCap == 0 {
					clock.Advance(time.Hour)
				}
				a.notify(fmt.Sprintf("key-%d", i), "x")
			}
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("notify blocked on a full queue")
		}
	})

	t.Run("nothing is sent after shutdown", func(t *testing.T) {
		f := newFakeTelegram(t)
		a := newTestAlerter(t, f, newFakeClock())

		drain(t, a)
		a.notify("k", "too late")

		if got := f.texts(); len(got) != 0 {
			t.Errorf("texts = %q, want none", got)
		}
	})

	t.Run("a nil alerter ignores alerts", func(t *testing.T) {
		var a *alerter
		a.notify("k", "nobody listens")
	})

	t.Run("a non-200 answer is an error", func(t *testing.T) {
		f := newFakeTelegram(t)
		f.status = http.StatusTooManyRequests
		a := newTestAlerter(t, f, newFakeClock())

		if err := a.send("x"); err == nil {
			t.Error("send returned nil for a 429 answer")
		}
	})
}

func installTestAlerter(t *testing.T, f *fakeTelegram) *alerter {
	t.Helper()
	a := newAlerter(testThread(f.server.URL), newFakeClock().Now)
	a.resource = testResource
	alerts.Store(a)
	t.Cleanup(func() {
		alerts.Store(nil)
		_ = a.shutdown(context.Background())
	})
	return a
}

func TestAlerts_FromReportErrorAndPanics(t *testing.T) {
	t.Run("an unexpected error alerts with its operation, route, version and request id", func(t *testing.T) {
		setupTestTelemetry(t)
		f := newFakeTelegram(t)
		a := installTestAlerter(t, f)

		rec := serve(newTestRouter(), "/fail", nil)
		drain(t, a)

		got := f.texts()
		if len(got) != 1 {
			t.Fatalf("texts = %q, want 1 message", got)
		}
		for _, want := range []string{"🔴 test · load profile", "route: /fail", "version: abc1234", "request: " + rec.Header().Get(RequestIDHeader)} {
			if !strings.Contains(got[0], want) {
				t.Errorf("text %q does not contain %q", got[0], want)
			}
		}
		if strings.Contains(got[0], secretSessionToken) {
			t.Errorf("text leaks a secret: %q", got[0])
		}
	})

	t.Run("a panic alerts with the scrubbed panic value", func(t *testing.T) {
		setupTestTelemetry(t)
		f := newFakeTelegram(t)
		a := installTestAlerter(t, f)

		serve(newTestRouter(), "/panic", nil)
		drain(t, a)

		got := f.texts()
		if len(got) != 1 {
			t.Fatalf("texts = %q, want 1 message", got)
		}
		if !strings.Contains(got[0], "💥 test · panic") || !strings.Contains(got[0], "route: /panic") || !strings.Contains(got[0], "boom") {
			t.Errorf("text = %q", got[0])
		}
		if strings.Contains(got[0], secretVerifier) {
			t.Errorf("text leaks a secret: %q", got[0])
		}
	})

	t.Run("repeated failures of one route alert once", func(t *testing.T) {
		setupTestTelemetry(t)
		f := newFakeTelegram(t)
		a := installTestAlerter(t, f)

		r := newTestRouter()
		for range 5 {
			serve(r, "/fail", nil)
		}
		drain(t, a)

		if got := f.texts(); len(got) != 1 {
			t.Errorf("got %d messages, want 1", len(got))
		}
	})

	t.Run("errors outside a request alert without route or request id", func(t *testing.T) {
		f := newFakeTelegram(t)
		a := installTestAlerter(t, f)

		ReportError(context.Background(), "background job failed", fmt.Errorf("send email: timeout"))
		drain(t, a)

		got := f.texts()
		if len(got) != 1 {
			t.Fatalf("texts = %q, want 1 message", got)
		}
		want := "🔴 test · background job failed\nerror: send email: timeout\nversion: abc1234"
		if got[0] != want {
			t.Errorf("text = %q, want %q", got[0], want)
		}
	})

	t.Run("long errors are cut", func(t *testing.T) {
		f := newFakeTelegram(t)
		a := installTestAlerter(t, f)

		ReportError(context.Background(), "op", fmt.Errorf("%s", strings.Repeat("é", alertMaxDetail+50)))
		drain(t, a)

		got := f.texts()
		if len(got) != 1 || !strings.Contains(got[0], strings.Repeat("é", alertMaxDetail)+"…\n") {
			t.Errorf("texts = %q, want the error cut to %d runes", got, alertMaxDetail)
		}
	})
}

func TestSetup_Alerts(t *testing.T) {
	t.Run("off without a chat id", func(t *testing.T) {
		tel, err := Setup(context.Background(), Options{Resource: testResource, Alerts: telegram.New(telegram.APIURL, testBotToken, "").Thread(testThreadID), Stdout: &syncBuffer{}})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		t.Cleanup(func() { _ = tel.Shutdown(context.Background()) })

		if alerts.Load() != nil {
			t.Error("alerts are on without a chat id")
		}
	})

	t.Run("on with a token and chat id, and off again after shutdown", func(t *testing.T) {
		tel, err := Setup(context.Background(), Options{
			Resource: testResource, Alerts: telegram.New(telegram.APIURL, testBotToken, testChatID).Thread(testThreadID), Stdout: &syncBuffer{},
		})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		if alerts.Load() == nil {
			t.Fatal("alerts are off with a token and chat id")
		}
		if err := tel.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown: %v", err)
		}
		if alerts.Load() != nil {
			t.Error("alerts are still on after shutdown")
		}
	})
}
