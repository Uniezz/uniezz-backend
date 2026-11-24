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
	if err := m.requestLimit.Allow(ctx, senderID.String()); err != nil {
		if errors.Is(err, ratelimit.ErrLimited) {
			return uuid.Nil, false, errors.Join(ErrRateLimited, err)
		}
		return uuid.Nil, false, err
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
