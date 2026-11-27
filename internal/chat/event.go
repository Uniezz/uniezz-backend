package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type eventName string

const (
	eventReady               eventName = "ready"
	eventMessageCreated      eventName = "message.created"
	eventMessageUpdated      eventName = "message.updated"
	eventConversationUpdated eventName = "conversation.updated"
	eventConversationRemoved eventName = "conversation.removed"
	eventReadUpdated         eventName = "read.updated"
	eventLocationUpdated     eventName = "location.updated"
	eventReconnect           eventName = "reconnect"
	eventAuthExpired         eventName = "auth_expired"
)

type messageEvent struct {
	ConversationID uuid.UUID `json:"conversationId"`
	ChangeSeq      int64     `json:"changeSeq"`
	Message        Message   `json:"message"`
}

type conversationEvent struct {
	ConversationID uuid.UUID `json:"conversationId"`
	ChangeSeq      int64     `json:"changeSeq"`
}

type conversationRemovedEvent struct {
	ConversationID uuid.UUID `json:"conversationId"`
}

type readEvent struct {
	ConversationID uuid.UUID `json:"conversationId"`
	UserID         uuid.UUID `json:"userId"`
	LastReadSeq    int64     `json:"lastReadSeq"`
	UnreadCount    *int      `json:"unreadCount,omitempty"`
}

type locationEvent struct {
	ConversationID uuid.UUID `json:"conversationId"`
	MessageID      uuid.UUID `json:"messageId"`
	Lat            float64   `json:"lat"`
	Lng            float64   `json:"lng"`
	Accuracy       *int      `json:"accuracy,omitempty"`
	Heading        *int      `json:"heading,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type reconnectEvent struct {
	AfterMs int64 `json:"afterMs"`
}

func encodeEvent(name eventName, data any) []byte {
	raw, err := json.Marshal(data)
	if err != nil {
		raw = []byte("{}")
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "event: %s\ndata: %s\n\n", name, raw)
	return b.Bytes()
}

func reconnectFrame(after time.Duration) []byte {
	return encodeEvent(eventReconnect, reconnectEvent{AfterMs: after.Milliseconds()})
}

var (
	readyFrame       = encodeEvent(eventReady, struct{}{})
	authExpiredFrame = encodeEvent(eventAuthExpired, struct{}{})
	heartbeatFrame   = []byte(": ping\n\n")
)

type notification struct {
	Event          eventName
	Recipients     []uuid.UUID
	ConversationID uuid.UUID
	ChangeSeq      int64
	Message        *messageRow
	UserID         uuid.UUID
	LastReadSeq    int64
	UnreadCount    *int
	Hydrated       *Message
}
