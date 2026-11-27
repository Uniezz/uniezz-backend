package chat

import (
	"context"
	"errors"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/google/uuid"
)

const (
	adultAge              = 18
	newAccountAge         = 48 * time.Hour
	maxNewAccountRequests = 5
)

type requestChange struct {
	RecipientID uuid.UUID
	Body        string
	Key         string
}

func (m Module) createRequest(ctx context.Context, senderID uuid.UUID, c requestChange) (*Conversation, bool, error) {
	in, err := sendChange{Body: c.Body, Key: c.Key}.clean()
	if err != nil {
		return nil, false, err
	}
	if senderID == c.RecipientID {
		return nil, false, ErrInvalidInput
	}

	conversation, err := m.replayRequest(ctx, senderID, in)
	if !errors.Is(err, ErrNotFound) {
		return conversation, err == nil, err
	}

	var created *messageRow
	var notes []notification
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		connected, err := m.members.connected(ctx, senderID, []uuid.UUID{c.RecipientID})
		if err != nil {
			return err
		}
		if len(connected) > 0 {
			return ErrDirectExists
		}
		if _, _, err := m.conversations.findDirect(ctx, senderID, c.RecipientID); err == nil {
			return ErrDirectExists
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		in.ConversationID, _, err = m.startRequest(ctx, senderID, c.RecipientID)
		if err != nil {
			return err
		}
		created, notes, err = m.storeMessage(ctx, senderID, in)
		return err
	})
	if errors.Is(err, ErrDirectExists) {
		if conversation, replayErr := m.replayRequest(ctx, senderID, in); replayErr == nil {
			return conversation, true, nil
		}
	}
	if errors.Is(err, errReplayed) || errors.Is(err, errClientTaken) {
		conversation, err := m.replayRequest(ctx, senderID, in)
		return conversation, err == nil, err
	}
	if err != nil {
		return nil, false, err
	}

	if _, err := m.deliverMessage(ctx, created, notes); err != nil {
		return nil, false, err
	}
	conversation, err = m.getConversation(ctx, senderID, in.ConversationID)
	return conversation, false, err
}

func (m Module) replayRequest(ctx context.Context, senderID uuid.UUID, in sendInput) (*Conversation, error) {
	row, err := m.messages.byClient(ctx, senderID, in.Key)
	if err != nil {
		return nil, err
	}
	in.ConversationID = row.ConversationID
	if _, err := m.replayMessage(ctx, senderID, in); err != nil {
		return nil, err
	}
	return m.getConversation(ctx, senderID, row.ConversationID)
}

func (m Module) startRequest(ctx context.Context, senderID, recipientID uuid.UUID) (uuid.UUID, bool, error) {
	allowed, err := m.requests.canStart(ctx, requestFilter{
		SenderID:    senderID,
		RecipientID: recipientID,
		AdultAge:    adultAge,
	})
	if err != nil {
		return uuid.Nil, false, err
	}
	if !allowed {
		return uuid.Nil, false, ErrNotFound
	}

	id, created, err := m.conversations.openDirect(ctx, senderID, recipientID, OriginRequest, StatusPending)
	if err != nil || !created {
		return id, created, err
	}
	if err := m.members.insert(ctx, id, []uuid.UUID{senderID, recipientID}, RoleMember, 0); err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

func (m Module) checkRequest(ctx context.Context, senderID uuid.UUID, a *accessRow, seq int64, in sendInput) error {
	if a.Kind != KindDirect || a.Status == StatusApproved {
		return nil
	}
	if a.Connected || a.StartedBy == nil || *a.StartedBy != senderID {
		return m.conversations.approve(ctx, in.ConversationID)
	}
	if seq > 1 || len(in.MediaIDs) > 0 || hasLink(in.Body) {
		return ErrRequestPending
	}

	// shortcut: concurrent first messages from several new accounts can pass the cap together; lock the recipient if that matters.
	inbox, err := m.requests.newAccountInbox(ctx, in.ConversationID, senderID, newAccountAge)
	if err != nil {
		return err
	}
	if inbox.SenderIsNew && inbox.Pending >= maxNewAccountRequests {
		return ErrRateLimited
	}
	if err := m.requestLimit.Allow(ctx, senderID.String()); err != nil {
		if errors.Is(err, ratelimit.ErrLimited) {
			return errors.Join(ErrRateLimited, err)
		}
		return err
	}
	return nil
}

func (m Module) acceptRequest(ctx context.Context, viewerID, id uuid.UUID) (*Conversation, error) {
	if err := m.requests.accept(ctx, viewerID, id); err != nil {
		return nil, err
	}
	conversation, err := m.getConversation(ctx, viewerID, id)
	if err != nil {
		return nil, err
	}
	if conversation.Incoming == nil || !*conversation.Incoming || *conversation.Status != StatusApproved {
		return nil, ErrNotFound
	}
	return conversation, nil
}

func (m Module) declineRequest(ctx context.Context, viewerID, id uuid.UUID) error {
	return m.requests.decline(ctx, viewerID, id)
}
