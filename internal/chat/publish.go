package chat

import (
	"context"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
)

func (m Module) publish(ctx context.Context, notes []notification) {
	if len(notes) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	defer cancel()

	views, err := m.hydrate(ctx, notes)
	if err != nil {
		telemetry.ReportError(ctx, "hydrate chat events", err, slog.Int("events", len(notes)))
	}

	for _, n := range notes {
		frame, ok := frameOf(n, views)
		if !ok {
			continue
		}
		m.hub.publish(n.Recipients, frame)
	}
}

func (m Module) hydrate(ctx context.Context, notes []notification) (map[uuid.UUID]Message, error) {
	views := map[uuid.UUID]Message{}
	var rows []messageRow
	for _, n := range notes {
		if n.Message == nil {
			continue
		}
		if n.Hydrated != nil {
			views[n.Message.ID] = *n.Hydrated
			continue
		}
		if _, done := views[n.Message.ID]; done {
			continue
		}
		views[n.Message.ID] = Message{}
		rows = append(rows, *n.Message)
	}
	if len(rows) == 0 {
		return views, nil
	}

	hydrated, err := m.messageViews(ctx, rows)
	if err != nil {
		return views, err
	}
	for _, v := range hydrated {
		views[v.ID] = v
	}
	return views, nil
}

func frameOf(n notification, views map[uuid.UUID]Message) ([]byte, bool) {
	switch n.Event {
	case eventMessageCreated, eventMessageUpdated:
		view, ok := views[n.Message.ID]
		if !ok || view.ID == uuid.Nil {
			return nil, false
		}
		return encodeEvent(n.Event, messageEvent{ConversationID: n.ConversationID, ChangeSeq: n.ChangeSeq, Message: view}), true
	case eventConversationUpdated:
		return encodeEvent(n.Event, conversationEvent{ConversationID: n.ConversationID, ChangeSeq: n.ChangeSeq}), true
	case eventConversationRemoved:
		return encodeEvent(n.Event, conversationRemovedEvent{ConversationID: n.ConversationID}), true
	case eventReadUpdated:
		return encodeEvent(n.Event, readEvent{ConversationID: n.ConversationID, UserID: n.UserID, LastReadSeq: n.LastReadSeq, UnreadCount: n.UnreadCount}), true
	}
	return nil, false
}
