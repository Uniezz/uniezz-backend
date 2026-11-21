package chat

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

type systemEvent string

const (
	systemGroupCreated systemEvent = "group_created"
	systemRenamed      systemEvent = "renamed"
	systemMemberAdded  systemEvent = "member_added"
	systemMemberRemove systemEvent = "member_removed"
	systemMemberLeft   systemEvent = "member_left"
	systemOwnerChanged systemEvent = "owner_changed"
)

type systemPayload struct {
	Event   systemEvent    `json:"event"`
	ActorID uuid.UUID      `json:"actorId"`
	Params  map[string]any `json:"params"`
}

func (m Module) writeSystemMessage(ctx context.Context, conversationID, actorID uuid.UUID, event systemEvent, params map[string]any, clientID *uuid.UUID) (*messageRow, error) {
	seq, changeSeq, err := m.conversations.bumpMessage(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(systemPayload{Event: event, ActorID: actorID, Params: params})
	if err != nil {
		return nil, err
	}
	row, err := m.messages.insert(ctx, newMessage{
		ConversationID: conversationID,
		Seq:            seq,
		ChangeSeq:      changeSeq,
		SenderID:       &actorID,
		Kind:           MessageSystem,
		Payload:        payload,
		ClientID:       clientID,
	})
	if err != nil {
		return nil, err
	}
	if err := m.members.countUnread(ctx, conversationID, actorID); err != nil {
		return nil, err
	}
	if err := m.members.markReadThrough(ctx, conversationID, actorID, seq); err != nil {
		return nil, err
	}
	return row, nil
}

func (m Module) membershipNotifications(ctx context.Context, conversationID, actorID uuid.UUID, rows ...*messageRow) ([]notification, error) {
	recipients, err := m.members.recipients(ctx, conversationID, actorID)
	if err != nil {
		return nil, err
	}
	notes := make([]notification, 0, len(rows)+1)
	for _, row := range rows {
		notes = append(notes, notification{Event: eventMessageCreated, Recipients: recipients, ConversationID: conversationID, ChangeSeq: row.ChangeSeq, Message: row})
	}
	last := rows[len(rows)-1]
	notes = append(notes, notification{Event: eventConversationUpdated, Recipients: recipients, ConversationID: conversationID, ChangeSeq: last.ChangeSeq})
	return notes, nil
}
