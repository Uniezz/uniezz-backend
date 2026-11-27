package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"
)

func (in sendInput) withCard(kind MessageKind, now time.Time, card any) (sendInput, error) {
	var payload any
	switch c := card.(type) {
	case *pollChange:
		p, err := c.payload(now)
		if err != nil {
			return sendInput{}, err
		}
		in.Body = p.Question
		payload = p
	case *eventChange:
		p, err := c.payload(now)
		if err != nil {
			return sendInput{}, err
		}
		in.Body = p.Title
		payload = p
	case *locationChange:
		p, err := c.payload()
		if err != nil {
			return sendInput{}, err
		}
		in.Body = p.body()
		payload = p
	case *cardChange:
		p, err := c.payload()
		if err != nil {
			return sendInput{}, err
		}
		in.Body = cardBodies[p.ItemType]
		payload = p
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return sendInput{}, fmt.Errorf("encode card: %w", err)
	}
	in.Kind = kind
	in.Payload = raw
	return in, nil
}

var mutableCardFields = []string{"closedAt", "canceledAt", "live", "lat", "lng"}

func sameCard(stored, sent []byte) bool {
	var a, b map[string]any
	if json.Unmarshal(stored, &a) != nil || json.Unmarshal(sent, &b) != nil {
		return false
	}
	for _, key := range mutableCardFields {
		delete(a, key)
		delete(b, key)
	}
	return reflect.DeepEqual(a, b)
}

func (m Module) lockCard(ctx context.Context, viewerID, messageID uuid.UUID, kind MessageKind) (*messageRow, *accessRow, error) {
	row, err := m.messages.getVisible(ctx, viewerID, messageID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := m.conversations.lockAsMember(ctx, row.ConversationID, viewerID); err != nil {
		return nil, nil, err
	}
	a, err := m.conversations.access(ctx, viewerID, row.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	if err := a.viewable(); err != nil {
		return nil, nil, err
	}
	row, err = m.messages.get(ctx, messageID)
	if err != nil {
		return nil, nil, err
	}
	if row.Kind != kind || row.DeletedAt != nil {
		return nil, nil, ErrNotFound
	}
	return row, a, nil
}

func (m Module) clearCard(ctx context.Context, row *messageRow) error {
	switch row.Kind {
	case MessagePoll:
		return m.polls.deleteVotes(ctx, row.ID)
	case MessageEvent:
		return m.events.deleteAnswers(ctx, row.ID)
	case MessageLocation:
		return m.lives.delete(ctx, row.ID)
	}
	return nil
}
