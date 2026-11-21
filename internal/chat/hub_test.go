package chat

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func closed(s *subscriber) bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func TestHubFansOutToEveryStreamOfAUser(t *testing.T) {
	h := newHub()
	user, other := uuid.New(), uuid.New()
	first, _ := h.subscribe(user, uuid.New())
	second, _ := h.subscribe(user, uuid.New())
	unrelated, _ := h.subscribe(other, uuid.New())

	h.publish([]uuid.UUID{user, uuid.New()}, []byte("frame"))

	for name, s := range map[string]*subscriber{"first": first, "second": second} {
		select {
		case got := <-s.events:
			if string(got) != "frame" {
				t.Errorf("%s got %q", name, got)
			}
		default:
			t.Errorf("%s stream received nothing", name)
		}
	}
	if len(unrelated.events) != 0 {
		t.Error("a stream of another user received the event")
	}
}

func TestHubClosesOnlyTheSubscriberWithAFullQueue(t *testing.T) {
	h := newHub()
	h.queueSize = 2
	slowUser, fastUser := uuid.New(), uuid.New()
	slow, _ := h.subscribe(slowUser, uuid.New())
	fast, _ := h.subscribe(fastUser, uuid.New())

	for i := range 3 {
		h.publish([]uuid.UUID{slowUser, fastUser}, []byte{byte(i)})
		<-fast.events
	}

	if !closed(slow) {
		t.Error("the stream that fell behind must be closed, not silently skipped")
	}
	if closed(fast) {
		t.Error("the stream that kept up must stay open")
	}
	if len(slow.events) != 2 {
		t.Errorf("queued = %d, want the 2 events that fit", len(slow.events))
	}
}

func TestHubSixthStreamClosesTheOldest(t *testing.T) {
	h := newHub()
	user := uuid.New()
	var open []*subscriber
	for range streamsPerUser {
		s, err := h.subscribe(user, uuid.New())
		if err != nil {
			t.Fatal(err)
		}
		open = append(open, s)
	}

	sixth, err := h.subscribe(user, uuid.New())
	if err != nil {
		t.Fatalf("a sixth stream must be accepted: %v", err)
	}
	if !closed(open[0]) {
		t.Error("the oldest stream must be closed")
	}
	for _, s := range append(open[1:], sixth) {
		if closed(s) {
			t.Error("a newer stream was closed")
		}
	}
	if h.open() != streamsPerUser {
		t.Errorf("open = %d, want %d", h.open(), streamsPerUser)
	}

	h.unsubscribe(open[0])
	if h.open() != streamsPerUser {
		t.Errorf("closing the evicted stream changed the count to %d", h.open())
	}
	h.unsubscribe(sixth)
	if h.open() != streamsPerUser-1 {
		t.Errorf("open = %d after one unsubscribe, want %d", h.open(), streamsPerUser-1)
	}
}

func TestHubRefusesStreamsAboveTheProcessCap(t *testing.T) {
	h := newHub()
	h.maxTotal = 3
	for range 3 {
		if _, err := h.subscribe(uuid.New(), uuid.New()); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := h.subscribe(uuid.New(), uuid.New()); !errors.Is(err, ErrTooManyStreams) {
		t.Errorf("err = %v, want ErrTooManyStreams", err)
	}
}

func TestHubDrainTellsEveryStreamToReconnect(t *testing.T) {
	h := newHub()
	h.reconnectIn = func() time.Duration { return 1500 * time.Millisecond }
	a, _ := h.subscribe(uuid.New(), uuid.New())
	b, _ := h.subscribe(uuid.New(), uuid.New())

	h.drain(reconnectFrame)

	for _, s := range []*subscriber{a, b} {
		if !closed(s) {
			t.Error("a drained stream must be closed")
		}
		got := string(<-s.events)
		if !strings.HasPrefix(got, "event: reconnect\n") || !strings.Contains(got, `"afterMs":1500`) {
			t.Errorf("frame = %q", got)
		}
	}
}

func TestHubReconnectDelayStaysWithinFiveSeconds(t *testing.T) {
	h := newHub()
	for range 1000 {
		if d := h.reconnectIn(); d < 0 || d > maxReconnectDelay {
			t.Fatalf("delay %v is outside 0 to %v", d, maxReconnectDelay)
		}
	}
}
