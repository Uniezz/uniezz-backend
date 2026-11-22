package chat

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"
)

func (m Module) ForgetUser(ctx context.Context, userID uuid.UUID) error {
	messageIDs, err := m.messages.liveIDsBySender(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range messageIDs {
		if err := ignoreGone(m.forgetMessage(ctx, id)); err != nil {
			return err
		}
	}

	directIDs, err := m.conversations.idsOfUser(ctx, userID, KindDirect)
	if err != nil {
		return err
	}
	for _, id := range directIDs {
		if err := ignoreGone(m.forgetDirect(ctx, userID, id)); err != nil {
			return err
		}
	}

	groupIDs, err := m.conversations.idsOfUser(ctx, userID, KindGroup)
	if err != nil {
		return err
	}
	for _, id := range groupIDs {
		if err := ignoreGone(m.leaveConversation(ctx, userID, id)); err != nil {
			return err
		}
	}
	return nil
}

func (m Module) forgetMessage(ctx context.Context, id uuid.UUID) error {
	row, err := m.messages.get(ctx, id)
	if err != nil {
		return err
	}

	var released []uuid.UUID
	var notes []notification
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.conversations.lock(ctx, row.ConversationID); err != nil {
			return err
		}
		var err error
		released, notes, err = m.tombstone(ctx, id, nil)
		return err
	})
	if err != nil {
		return err
	}

	m.releaseMedia(ctx, released)
	m.publish(ctx, notes)
	return nil
}

func (m Module) forgetDirect(ctx context.Context, userID, conversationID uuid.UUID) error {
	var released []uuid.UUID
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.conversations.lock(ctx, conversationID); err != nil {
			return err
		}
		memberIDs, err := m.members.userIDs(ctx, conversationID)
		if err != nil {
			return err
		}
		others := slices.DeleteFunc(memberIDs, func(id uuid.UUID) bool { return id == userID })
		removed := notification{Event: eventConversationRemoved, Recipients: others, ConversationID: conversationID}
		released, notes, err = m.deleteConversation(ctx, conversationID, removed)
		return err
	})
	if err != nil {
		return err
	}

	m.releaseMedia(ctx, released)
	m.publish(ctx, notes)
	return nil
}

func ignoreGone(err error) error {
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
