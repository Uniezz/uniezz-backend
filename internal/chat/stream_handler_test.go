package chat

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type sseEvent struct {
	name string
	data string
}

type sseStream struct {
	lines  chan string
	events chan sseEvent
	body   *http.Response
}

func (e *testEnv) streamServer(t *testing.T, writeTimeout time.Duration, mutate func(*Module)) *httptest.Server {
	t.Helper()
	module := e.module
	if mutate != nil {
		mutate(&module)
	}

	r := chi.NewRouter()
	api := humachi.New(r, httpapi.NewConfig(&config.Config{}))
	api.UseMiddleware(e.auth.Guard(api))
	module.Register(api)

	srv := httptest.NewUnstartedServer(r)
	if writeTimeout > 0 {
		srv.Config.WriteTimeout = writeTimeout
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv
}

func openSSE(t *testing.T, srv *httptest.Server, u testUser) *sseStream {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/chat/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+u.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q", got)
	}

	s := &sseStream{lines: make(chan string, 1024), events: make(chan sseEvent, 1024), body: resp}
	go func() {
		defer close(s.lines)
		defer close(s.events)
		scanner := bufio.NewScanner(resp.Body)
		var current sseEvent
		for scanner.Scan() {
			line := scanner.Text()
			s.lines <- line
			switch {
			case strings.HasPrefix(line, "event: "):
				current.name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				current.data = strings.TrimPrefix(line, "data: ")
			case line == "" && current.name != "":
				s.events <- current
				current = sseEvent{}
			}
		}
	}()
	return s
}

func (s *sseStream) next(t *testing.T, within time.Duration) sseEvent {
	t.Helper()
	select {
	case ev, ok := <-s.events:
		if !ok {
			t.Fatal("stream closed while waiting for an event")
		}
		return ev
	case <-time.After(within):
		t.Fatalf("no event within %v", within)
	}
	return sseEvent{}
}

func (s *sseStream) expect(t *testing.T, name string, within time.Duration) sseEvent {
	t.Helper()
	ev := s.next(t, within)
	if ev.name != name {
		t.Fatalf("event = %s (%s), want %s", ev.name, ev.data, name)
	}
	return ev
}

func (s *sseStream) closes(within time.Duration) bool {
	timeout := time.After(within)
	for {
		select {
		case _, ok := <-s.lines:
			if !ok {
				return true
			}
		case <-timeout:
			return false
		}
	}
}

func TestStreamDeliversEventsAfterWrites(t *testing.T) {
	e := newTestEnv(t)
	srv := e.streamServer(t, 0, nil)
	a, b := e.friends(t)
	conversation := e.direct(t, a, b)

	streamA, streamB := openSSE(t, srv, a), openSSE(t, srv, b)
	streamA.expect(t, "ready", time.Second)
	streamB.expect(t, "ready", time.Second)

	key := uuid.New()
	sent := decode[Message](t, e.rawSend(a, conversation.ID, key, map[string]any{"body": "live"}), http.StatusCreated)

	for name, s := range map[string]*sseStream{"sender": streamA, "recipient": streamB} {
		ev := s.expect(t, "message.created", time.Second)
		var payload messageEvent
		if err := json.Unmarshal([]byte(ev.data), &payload); err != nil {
			t.Fatalf("%s: invalid event data %q: %v", name, ev.data, err)
		}
		if payload.Message.ID != sent.ID || payload.ChangeSeq != sent.ChangeSeq || payload.ConversationID != conversation.ID {
			t.Errorf("%s got %+v, want message %s", name, payload, sent.ID)
		}
		if payload.Message.ClientID == nil || *payload.Message.ClientID != key {
			t.Errorf("%s got clientId %v, want %s", name, payload.Message.ClientID, key)
		}
	}
}

func TestStreamSendsHeartbeatsAndOutlivesTheWriteTimeout(t *testing.T) {
	e := newTestEnv(t)
	srv := e.streamServer(t, 300*time.Millisecond, func(m *Module) { m.heartbeat = 50 * time.Millisecond })
	a := e.user(t)

	stream := openSSE(t, srv, a)
	stream.expect(t, "ready", time.Second)

	pings := 0
	deadline := time.After(1500 * time.Millisecond)
	for pings < 20 {
		select {
		case line, ok := <-stream.lines:
			if !ok {
				t.Fatalf("stream closed after %d pings, the write deadline must be extended on every write", pings)
			}
			if line == ": ping" {
				pings++
			}
		case <-deadline:
			t.Fatalf("only %d pings in 1.5s", pings)
		}
	}
}

func TestStreamEndsWithAuthExpiredWhenTheSessionEnds(t *testing.T) {
	e := newTestEnv(t)
	srv := e.streamServer(t, 0, func(m *Module) { m.sessionCheck = 100 * time.Millisecond })
	a := e.user(t)

	stream := openSSE(t, srv, a)
	stream.expect(t, "ready", time.Second)
	if _, err := e.pool.Exec(context.Background(), "DELETE FROM sessions WHERE id = $1", a.sessionID); err != nil {
		t.Fatal(err)
	}

	stream.expect(t, "auth_expired", 2*time.Second)
	if !stream.closes(time.Second) {
		t.Error("the stream must close after auth_expired")
	}
	if got := e.module.hub.open(); got != 0 {
		t.Errorf("open streams = %d, want 0", got)
	}
}

func TestStreamEndsWithAuthExpiredWhenTheStudentIsBanned(t *testing.T) {
	e := newTestEnv(t)
	srv := e.streamServer(t, 0, func(m *Module) { m.sessionCheck = 100 * time.Millisecond })
	a := e.user(t)

	stream := openSSE(t, srv, a)
	stream.expect(t, "ready", time.Second)
	if _, err := e.pool.Exec(context.Background(), "UPDATE users SET banned_at = NOW() WHERE id = $1", a.id); err != nil {
		t.Fatal(err)
	}
	stream.expect(t, "auth_expired", 2*time.Second)
}

func TestStreamReceivesReconnectOnDrain(t *testing.T) {
	e := newTestEnv(t)
	srv := e.streamServer(t, 0, nil)
	a := e.user(t)

	stream := openSSE(t, srv, a)
	stream.expect(t, "ready", time.Second)
	e.module.Drain()

	ev := stream.expect(t, "reconnect", time.Second)
	var payload reconnectEvent
	if err := json.Unmarshal([]byte(ev.data), &payload); err != nil || payload.AfterMs < 0 || payload.AfterMs > maxReconnectDelay.Milliseconds() {
		t.Errorf("reconnect data = %q err = %v", ev.data, err)
	}
	if !stream.closes(time.Second) {
		t.Error("the stream must close after reconnect")
	}
}

func TestStreamRefusals(t *testing.T) {
	e := newTestEnv(t)

	t.Run("no session answers a JSON 401", func(t *testing.T) {
		srv := e.streamServer(t, 0, nil)
		resp, err := http.Get(srv.URL + "/chat/stream")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			t.Errorf("status = %d content type = %q", resp.StatusCode, resp.Header.Get("Content-Type"))
		}
	})

	t.Run("a full process answers 503", func(t *testing.T) {
		module := e.module
		module.hub = newHub()
		module.hub.maxTotal = 0
		r := chi.NewRouter()
		api := humachi.New(r, httpapi.NewConfig(&config.Config{}))
		api.UseMiddleware(e.auth.Guard(api))
		module.Register(api)
		srv := httptest.NewServer(r)
		t.Cleanup(srv.Close)

		a := e.user(t)
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/chat/stream", nil)
		req.Header.Set("Authorization", "Bearer "+a.token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want 503", resp.StatusCode)
		}
	})
}

func (e *testEnv) subscribe(t *testing.T, u testUser) *subscriber {
	t.Helper()
	s, err := e.module.hub.subscribe(u.id, u.sessionID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.module.hub.unsubscribe(s) })
	return s
}

func eventNames(s *subscriber) []string {
	var names []string
	for {
		select {
		case frame := <-s.events:
			line, _, _ := strings.Cut(string(frame), "\n")
			names = append(names, strings.TrimPrefix(line, "event: "))
		default:
			return names
		}
	}
}

func TestEveryWritePublishesToTheRightStudents(t *testing.T) {
	e := newTestEnv(t)

	t.Run("send, delete and read in a direct conversation", func(t *testing.T) {
		a, b := e.friends(t)
		conversation := e.direct(t, a, b)
		streamA, streamB := e.subscribe(t, a), e.subscribe(t, b)

		sent := e.send(t, a, conversation.ID, "hello")
		if got := eventNames(streamA); len(got) != 1 || got[0] != "message.created" {
			t.Errorf("sender events = %v", got)
		}
		if got := eventNames(streamB); len(got) != 1 || got[0] != "message.created" {
			t.Errorf("recipient events = %v", got)
		}

		decode[struct{ LastReadSeq int64 }](t, e.api.Put("/chat/conversations/"+conversation.ID.String()+"/read", b.bearer, map[string]any{"seq": sent.Seq}), http.StatusOK)
		readerFrames, peerFrames := drainFrames(streamB), drainFrames(streamA)
		if len(readerFrames) != 1 || !strings.Contains(readerFrames[0], "event: read.updated") || !strings.Contains(readerFrames[0], `"unreadCount":0`) {
			t.Errorf("reader frames = %q", readerFrames)
		}
		if len(peerFrames) != 1 || !strings.Contains(peerFrames[0], "event: read.updated") || strings.Contains(peerFrames[0], "unreadCount") {
			t.Errorf("peer frames = %q, want read.updated without the reader's unread count", peerFrames)
		}

		decode[map[string]any](t, e.api.Put("/chat/conversations/"+conversation.ID.String()+"/read", b.bearer, map[string]any{"seq": sent.Seq}), http.StatusOK)
		if got := eventNames(streamB); len(got) != 0 {
			t.Errorf("reading nothing new published %v", got)
		}

		if rec := e.api.Delete("/chat/messages/"+sent.ID.String(), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d", rec.Code)
		}
		for name, s := range map[string]*subscriber{"sender": streamA, "recipient": streamB} {
			if got := eventNames(s); len(got) != 1 || got[0] != "message.updated" {
				t.Errorf("%s events after delete = %v", name, got)
			}
		}
	})

	t.Run("a blocked student does not receive a group member's message", func(t *testing.T) {
		owner, b := e.friends(t)
		c := e.befriend(t, owner)
		group := e.group(t, owner, "Group", b, c)
		e.block(t, b, c)
		streamB, streamC, streamOwner := e.subscribe(t, b), e.subscribe(t, c), e.subscribe(t, owner)

		e.send(t, c, group.ID, "from c")
		if got := eventNames(streamB); len(got) != 0 {
			t.Errorf("b received %v from a blocked student", got)
		}
		if got := eventNames(streamC); len(got) != 1 {
			t.Errorf("c events = %v, want its own message", got)
		}
		if got := eventNames(streamOwner); len(got) != 1 || got[0] != "message.created" {
			t.Errorf("owner events = %v", got)
		}
	})

	t.Run("group changes reach members and the removed student is told separately", func(t *testing.T) {
		owner, b := e.friends(t)
		c := e.befriend(t, owner)
		streamOwner, streamB, streamC := e.subscribe(t, owner), e.subscribe(t, b), e.subscribe(t, c)

		group := e.group(t, owner, "Group", b, c)
		for name, s := range map[string]*subscriber{"owner": streamOwner, "b": streamB, "c": streamC} {
			if got := eventNames(s); len(got) != 2 || got[0] != "message.created" || got[1] != "conversation.updated" {
				t.Errorf("%s events after creating = %v", name, got)
			}
		}

		if rec := e.api.Delete("/chat/conversations/"+group.ID.String()+"/members/"+c.id.String(), owner.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("remove: %d", rec.Code)
		}
		if got := eventNames(streamC); len(got) != 1 || got[0] != "conversation.removed" {
			t.Errorf("removed student events = %v", got)
		}
		if got := eventNames(streamB); len(got) != 2 || got[1] != "conversation.updated" {
			t.Errorf("remaining member events = %v", got)
		}
		drainFrames(streamOwner)

		if rec := e.api.Delete("/chat/conversations/"+group.ID.String()+"/members/me", b.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("leave: %d", rec.Code)
		}
		if got := eventNames(streamB); len(got) != 1 || got[0] != "conversation.removed" {
			t.Errorf("leaver events = %v", got)
		}
	})
}

func drainFrames(s *subscriber) []string {
	var frames []string
	for {
		select {
		case frame := <-s.events:
			frames = append(frames, string(frame))
		default:
			return frames
		}
	}
}

func TestDrainDoesNotWaitForAReaderThatStoppedReading(t *testing.T) {
	e := newTestEnv(t)
	srv := e.streamServer(t, 0, func(m *Module) { m.closeWait = 300 * time.Millisecond })
	a := e.user(t)

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if _, err := fmt.Fprintf(conn, "GET /chat/stream HTTP/1.1\r\nHost: test\r\nAuthorization: Bearer %s\r\n\r\n", a.token); err != nil {
		t.Fatal(err)
	}
	for e.module.hub.open() == 0 {
		time.Sleep(10 * time.Millisecond)
	}

	frame := make([]byte, 256<<10)
	for range streamQueueSize - 1 {
		e.module.hub.publish([]uuid.UUID{a.id}, frame)
	}
	time.Sleep(500 * time.Millisecond)
	e.module.Drain()

	deadline := time.Now().Add(5 * time.Second)
	for e.module.hub.open() > 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := e.module.hub.open(); got != 0 {
		t.Errorf("open streams = %d five seconds after Drain, a blocked write must not hold the stream", got)
	}
}

func TestStreamIsRefusedWhileDraining(t *testing.T) {
	e := newTestEnv(t)
	srv := e.streamServer(t, 0, nil)
	a := e.user(t)
	e.module.Drain()

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/chat/stream", nil)
	req.Header.Set("Authorization", "Bearer "+a.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429 so a deploy does not raise server error alerts", resp.StatusCode)
	}
}
