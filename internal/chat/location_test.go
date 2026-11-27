package chat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
)

func (e *testEnv) sendLocation(t *testing.T, u testUser, conversationID uuid.UUID, live bool) Message {
	t.Helper()
	body := map[string]any{"location": map[string]any{"lat": 51.24651234, "lng": 22.5684, "label": "UMCS gate", "live": live}}
	return decode[Message](t, e.rawSend(u, conversationID, uuid.New(), body), http.StatusCreated)
}

func (e *testEnv) rawMove(u testUser, messageID uuid.UUID, lat, lng float64) *httptest.ResponseRecorder {
	return e.api.Post("/chat/messages/"+messageID.String()+"/location", u.bearer, map[string]any{"lat": lat, "lng": lng, "accuracy": 12})
}

func (e *testEnv) backdateLive(t *testing.T, messageID uuid.UUID, column string, by time.Duration) {
	t.Helper()
	query := "UPDATE chat_live_locations SET " + column + " = " + column + " - $2::interval WHERE message_id = $1"
	if _, err := e.pool.Exec(context.Background(), query, messageID, by); err != nil {
		t.Fatal(err)
	}
}

func (e *testEnv) liveRows(t *testing.T, messageID uuid.UUID) int {
	t.Helper()
	var n int
	if err := e.pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM chat_live_locations WHERE message_id = $1", messageID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSendLocation(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a place keeps 6 decimals and uses its label as body", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		got := e.sendLocation(t, a, conversation.ID, false)
		if got.Kind != MessageLocation || got.Body == nil || *got.Body != "UMCS gate" || got.Location == nil {
			t.Fatalf("message = %+v", got)
		}
		if got.Location.Lat != 51.246512 || got.Location.Live != nil || e.liveRows(t, got.ID) != 0 {
			t.Errorf("location = %+v", got.Location)
		}
	})

	t.Run("a live location runs for one hour", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		got := e.sendLocation(t, a, conversation.ID, true)
		live := got.Location.Live
		if live == nil || live.StoppedAt != nil || live.Until.Sub(live.UpdatedAt) != time.Hour || e.liveRows(t, got.ID) != 1 {
			t.Errorf("live = %+v", live)
		}
	})

	t.Run("coordinates out of range are rejected", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		body := map[string]any{"location": map[string]any{"lat": 91, "lng": 0}}
		if rec := e.rawSend(a, conversation.ID, uuid.New(), body); rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("status = %d, want a client error", rec.Code)
		}
	})
}

func TestLiveLocation(t *testing.T) {
	e := newTestEnv(t)

	t.Run("the sender moves the position at most once per 10 seconds", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		sent := e.sendLocation(t, a, conversation.ID, true)

		expectError(t, e.rawMove(a, sent.ID, 51.3, 22.6), http.StatusTooManyRequests, "rate_limited")
		e.backdateLive(t, sent.ID, "moved_at", 11*time.Second)
		if rec := e.rawMove(a, sent.ID, 51.3, 22.6); rec.Code != http.StatusNoContent {
			t.Fatalf("move: %d %s", rec.Code, rec.Body.String())
		}
		expectError(t, e.rawMove(a, sent.ID, 51.4, 22.7), http.StatusTooManyRequests, "rate_limited")

		got := e.message(t, b, sent.ID).Location
		if got.Lat != 51.3 || got.Lng != 22.6 || got.Live.Accuracy == nil || *got.Live.Accuracy != 12 {
			t.Errorf("location = %+v live %+v", got, got.Live)
		}
	})

	t.Run("only the sender may move or stop it", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		sent := e.sendLocation(t, a, conversation.ID, true)
		e.backdateLive(t, sent.ID, "moved_at", 11*time.Second)

		expectError(t, e.rawMove(b, sent.ID, 51.3, 22.6), http.StatusForbidden, "forbidden")
		expectError(t, e.api.Post("/chat/messages/"+sent.ID.String()+"/location/stop", b.bearer), http.StatusForbidden, "forbidden")
		expectError(t, e.rawMove(e.user(t), sent.ID, 51.3, 22.6), http.StatusNotFound, "not_found")
	})

	t.Run("stopping keeps the last position and can be repeated", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		sent := e.sendLocation(t, a, conversation.ID, true)
		e.backdateLive(t, sent.ID, "moved_at", 11*time.Second)
		if rec := e.rawMove(a, sent.ID, 50.5, 21.5); rec.Code != http.StatusNoContent {
			t.Fatalf("move: %d", rec.Code)
		}

		stopped := decode[Message](t, e.api.Post("/chat/messages/"+sent.ID.String()+"/location/stop", a.bearer), http.StatusOK)
		loc := stopped.Location
		if loc.Lat != 50.5 || loc.Lng != 21.5 || loc.Live.StoppedAt == nil || e.liveRows(t, sent.ID) != 0 || stopped.ChangeSeq <= sent.ChangeSeq {
			t.Errorf("stopped = %+v live %+v", loc, loc.Live)
		}
		decode[Message](t, e.api.Post("/chat/messages/"+sent.ID.String()+"/location/stop", a.bearer), http.StatusOK)
		expectError(t, e.rawMove(a, sent.ID, 50.6, 21.6), http.StatusConflict, "location_not_live")
	})

	t.Run("the job ends expired shares and catch-up sees their last position", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		expired := e.sendLocation(t, a, conversation.ID, true)
		running := e.sendLocation(t, a, conversation.ID, true)
		e.backdateLive(t, expired.ID, "live_until", 2*time.Hour)

		expectError(t, e.rawMove(a, expired.ID, 50, 20), http.StatusConflict, "location_not_live")
		if err := e.module.finishLiveLocations(context.Background(), FinishLiveLocationsArgs{}); err != nil {
			t.Fatalf("finishLiveLocations: %v", err)
		}

		if e.liveRows(t, expired.ID) != 0 || e.liveRows(t, running.ID) != 1 {
			t.Errorf("live rows: expired %d running %d", e.liveRows(t, expired.ID), e.liveRows(t, running.ID))
		}
		page := e.history(t, b, conversation.ID, "changedAfter="+strconv.FormatInt(running.ChangeSeq, 10))
		if len(page.Items) != 1 || page.Items[0].ID != expired.ID || page.Items[0].Location.Live.StoppedAt == nil {
			t.Errorf("catch-up = %+v", page.Items)
		}
	})

	t.Run("deleting a live location removes its position", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		sent := e.sendLocation(t, a, conversation.ID, true)
		if rec := e.api.Delete("/chat/messages/"+sent.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", rec.Code)
		}
		if e.liveRows(t, sent.ID) != 0 {
			t.Errorf("live row left after delete")
		}
		expectError(t, e.rawMove(a, sent.ID, 50, 20), http.StatusNotFound, "not_found")
	})

	t.Run("moving and stopping without a session return 401", func(t *testing.T) {
		id := uuid.NewString()
		for _, rec := range []*httptest.ResponseRecorder{
			e.api.Post("/chat/messages/"+id+"/location", map[string]any{"lat": 1, "lng": 1}),
			e.api.Post("/chat/messages/" + id + "/location/stop"),
		} {
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want 401", rec.Code)
			}
		}
	})
}
