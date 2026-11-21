package chat

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
)

const (
	streamsPerUser    = 5
	streamsPerProcess = 5000
	streamQueueSize   = 256
	maxReconnectDelay = 5 * time.Second
)

type subscriber struct {
	userID    uuid.UUID
	sessionID uuid.UUID
	events    chan []byte
	done      chan struct{}
	once      sync.Once
	openedAt  time.Time
}

func (s *subscriber) close() {
	s.once.Do(func() { close(s.done) })
}

type hub struct {
	mu          sync.Mutex
	users       map[uuid.UUID][]*subscriber
	total       int
	perUser     int
	maxTotal    int
	queueSize   int
	now         func() time.Time
	reconnectIn func() time.Duration
}

func newHub() *hub {
	return &hub{
		users:     map[uuid.UUID][]*subscriber{},
		perUser:   streamsPerUser,
		maxTotal:  streamsPerProcess,
		queueSize: streamQueueSize,
		now:       time.Now,
		reconnectIn: func() time.Duration {
			return time.Duration(rand.Int64N(int64(maxReconnectDelay) + 1))
		},
	}
}

func (h *hub) subscribe(userID, sessionID uuid.UUID) (*subscriber, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.total >= h.maxTotal {
		return nil, ErrTooManyStreams
	}
	s := &subscriber{
		userID:    userID,
		sessionID: sessionID,
		events:    make(chan []byte, h.queueSize),
		done:      make(chan struct{}),
		openedAt:  h.now(),
	}
	open := h.users[userID]
	if len(open) >= h.perUser {
		open[0].close()
		h.total--
		open = slices.Clone(open[1:])
	}
	h.users[userID] = append(open, s)
	h.total++
	telemetry.RecordGauge(context.Background(), "chat.streams.open", float64(h.total))
	return s, nil
}

func (h *hub) unsubscribe(s *subscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()

	s.close()
	open := h.users[s.userID]
	i := slices.Index(open, s)
	if i < 0 {
		return
	}
	open = slices.Delete(slices.Clone(open), i, i+1)
	if len(open) == 0 {
		delete(h.users, s.userID)
	} else {
		h.users[s.userID] = open
	}
	h.total--
	telemetry.RecordGauge(context.Background(), "chat.streams.open", float64(h.total))
}

func (h *hub) publish(userIDs []uuid.UUID, frame []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var delivered, overflowed int64
	for _, userID := range userIDs {
		for _, s := range h.users[userID] {
			select {
			case <-s.done:
			case s.events <- frame:
				delivered++
			default:
				s.close()
				overflowed++
			}
		}
	}
	if delivered > 0 {
		telemetry.RecordCount(context.Background(), "chat.events.published", delivered)
	}
	if overflowed > 0 {
		telemetry.RecordCount(context.Background(), "chat.streams.overflowed", overflowed)
	}
}

func (h *hub) drain(reconnectFrame func(after time.Duration) []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var drained int64
	for _, open := range h.users {
		for _, s := range open {
			select {
			case s.events <- reconnectFrame(h.reconnectIn()):
			default:
			}
			s.close()
			drained++
		}
	}
	telemetry.RecordCount(context.Background(), "chat.streams.drained", drained)
}

func (h *hub) open() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.total
}
