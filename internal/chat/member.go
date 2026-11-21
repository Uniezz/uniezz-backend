package chat

import (
	"context"
	"slices"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

type memberQuery struct {
	ViewerID       uuid.UUID
	ConversationID uuid.UUID
	Cursor         string
	Limit          int
}

type addChange struct {
	ConversationID uuid.UUID
	UserIDs        []uuid.UUID
}

type removeChange struct {
	ConversationID uuid.UUID
	MemberID       uuid.UUID
}

type readChange struct {
	ConversationID uuid.UUID
	Seq            int64
}

func (m Module) listMembers(ctx context.Context, q memberQuery) ([]Member, *string, error) {
	a, err := m.conversations.access(ctx, q.ViewerID, q.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	if err := a.viewable(); err != nil {
		return nil, nil, err
	}

	filter := memberFilter{ConversationID: q.ConversationID, ViewerID: q.ViewerID, Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after memberCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.members.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last memberRow) any {
		return memberCursor{NotOwner: last.Role != RoleOwner, JoinedAt: last.JoinedAt, UserID: last.UserID}
	})
	if err != nil {
		return nil, nil, err
	}

	items, err := m.memberViews(ctx, rows)
	if err != nil {
		return nil, nil, err
	}
	return items, next, nil
}

func (m Module) memberViews(ctx context.Context, rows []memberRow) ([]Member, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserID)
	}
	students, err := m.studentMap(ctx, ids)
	if err != nil {
		return nil, err
	}

	items := make([]Member, 0, len(rows))
	for _, row := range rows {
		id := row.UserID
		items = append(items, Member{Student: studentOf(students, &id), Role: row.Role, JoinedAt: row.JoinedAt})
	}
	return items, nil
}

func (m Module) addMembers(ctx context.Context, userID uuid.UUID, c addChange) ([]Member, error) {
	if len(c.UserIDs) < 1 || len(c.UserIDs) > maxGroupMembers-1 || hasDuplicates(c.UserIDs) {
		return nil, ErrInvalidInput
	}

	var added []memberRow
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.conversations.lockAsMember(ctx, c.ConversationID, userID); err != nil {
			return err
		}
		a, err := m.conversations.access(ctx, userID, c.ConversationID)
		if err != nil {
			return err
		}
		if err := a.groupOnly(); err != nil {
			return err
		}

		existing, err := m.members.existing(ctx, c.ConversationID, c.UserIDs)
		if err != nil {
			return err
		}
		fresh := make([]uuid.UUID, 0, len(c.UserIDs))
		for _, id := range c.UserIDs {
			if !slices.Contains(existing, id) {
				fresh = append(fresh, id)
			}
		}
		if len(fresh) == 0 {
			return nil
		}

		connected, err := m.members.connected(ctx, userID, fresh)
		if err != nil {
			return err
		}
		if len(connected) != len(fresh) {
			return ErrInvalidMember
		}
		count, err := m.members.count(ctx, c.ConversationID)
		if err != nil {
			return err
		}
		if count+len(fresh) > maxGroupMembers {
			return ErrGroupFull
		}

		ids := make([]string, 0, len(fresh))
		for _, id := range fresh {
			ids = append(ids, id.String())
		}
		message, err := m.writeSystemMessage(ctx, c.ConversationID, userID, systemMemberAdded, map[string]any{"userIds": ids}, nil)
		if err != nil {
			return err
		}
		if err := m.members.insert(ctx, c.ConversationID, fresh, RoleMember, message.Seq); err != nil {
			return err
		}
		notes, err = m.membershipNotifications(ctx, c.ConversationID, userID, message)
		if err != nil {
			return err
		}
		for _, id := range fresh {
			added = append(added, memberRow{UserID: id, Role: RoleMember})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	m.publish(ctx, notes)
	return m.memberViews(ctx, added)
}

func (m Module) removeMember(ctx context.Context, userID uuid.UUID, c removeChange) error {
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.conversations.lockAsMember(ctx, c.ConversationID, userID); err != nil {
			return err
		}
		a, err := m.conversations.access(ctx, userID, c.ConversationID)
		if err != nil {
			return err
		}
		if err := a.groupOnly(); err != nil {
			return err
		}
		if err := a.ownerOnly(); err != nil {
			return err
		}
		if c.MemberID == userID {
			return ErrInvalidInput
		}

		if err := m.members.delete(ctx, c.ConversationID, c.MemberID); err != nil {
			return err
		}
		message, err := m.writeSystemMessage(ctx, c.ConversationID, userID, systemMemberRemove, map[string]any{"userId": c.MemberID.String()}, nil)
		if err != nil {
			return err
		}
		notes, err = m.membershipNotifications(ctx, c.ConversationID, userID, message)
		if err != nil {
			return err
		}
		notes = append(notes, notification{Event: eventConversationRemoved, Recipients: []uuid.UUID{c.MemberID}, ConversationID: c.ConversationID})
		return nil
	})
	if err != nil {
		return err
	}

	m.publish(ctx, notes)
	return nil
}

func (m Module) leaveConversation(ctx context.Context, userID, conversationID uuid.UUID) error {
	var released []uuid.UUID
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.conversations.lockAsMember(ctx, conversationID, userID); err != nil {
			return err
		}
		var err error
		released, notes, err = m.leaveGroup(ctx, userID, conversationID)
		return err
	})
	if err != nil {
		return err
	}

	m.releaseMedia(ctx, released)
	m.publish(ctx, notes)
	return nil
}

func (m Module) leaveGroup(ctx context.Context, userID, conversationID uuid.UUID) ([]uuid.UUID, []notification, error) {
	a, err := m.conversations.access(ctx, userID, conversationID)
	if err != nil {
		return nil, nil, err
	}
	if err := a.groupOnly(); err != nil {
		return nil, nil, err
	}

	count, err := m.members.count(ctx, conversationID)
	if err != nil {
		return nil, nil, err
	}
	removed := notification{Event: eventConversationRemoved, Recipients: []uuid.UUID{userID}, ConversationID: conversationID}
	if count == 1 {
		return m.deleteConversation(ctx, conversationID, removed)
	}

	if err := m.members.delete(ctx, conversationID, userID); err != nil {
		return nil, nil, err
	}
	left, err := m.writeSystemMessage(ctx, conversationID, userID, systemMemberLeft, map[string]any{"userId": userID.String()}, nil)
	if err != nil {
		return nil, nil, err
	}
	messages := []*messageRow{left}

	if a.Role == RoleOwner {
		next, err := m.members.longestStanding(ctx, conversationID)
		if err != nil {
			return nil, nil, err
		}
		if err := m.members.setRole(ctx, conversationID, next, RoleOwner); err != nil {
			return nil, nil, err
		}
		changed, err := m.writeSystemMessage(ctx, conversationID, userID, systemOwnerChanged, map[string]any{"userId": next.String()}, nil)
		if err != nil {
			return nil, nil, err
		}
		messages = append(messages, changed)
	}

	notes, err := m.membershipNotifications(ctx, conversationID, userID, messages...)
	if err != nil {
		return nil, nil, err
	}
	return nil, append(notes, removed), nil
}

func (m Module) deleteConversation(ctx context.Context, conversationID uuid.UUID, removed notification) ([]uuid.UUID, []notification, error) {
	released, err := m.messages.conversationMediaIDs(ctx, conversationID)
	if err != nil {
		return nil, nil, err
	}
	if len(released) > 0 {
		if err := m.media.MarkDeletingMany(ctx, released); err != nil {
			return nil, nil, err
		}
	}
	if err := m.conversations.delete(ctx, conversationID); err != nil {
		return nil, nil, err
	}
	return released, []notification{removed}, nil
}

func (m Module) markRead(ctx context.Context, userID uuid.UUID, c readChange) (*readRow, error) {
	var read *readRow
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		locked, err := m.conversations.lockAsMember(ctx, c.ConversationID, userID)
		if err != nil {
			return err
		}
		a, err := m.conversations.access(ctx, userID, c.ConversationID)
		if err != nil {
			return err
		}
		if err := a.viewable(); err != nil {
			return err
		}
		if c.Seq < 0 || c.Seq > locked.LastSeq {
			return ErrInvalidInput
		}

		read, err = m.members.markRead(ctx, c.ConversationID, userID, c.Seq, locked.LastSeq)
		if err != nil || read.LastReadSeq <= read.PreviousSeq {
			return err
		}
		notes, err = m.readNotifications(ctx, locked, userID, read)
		return err
	})
	if err != nil {
		return nil, err
	}

	m.publish(ctx, notes)
	return read, nil
}

func (m Module) readNotifications(ctx context.Context, conversation *lockedRow, userID uuid.UUID, read *readRow) ([]notification, error) {
	unread := read.UnreadCount
	notes := []notification{{
		Event: eventReadUpdated, Recipients: []uuid.UUID{userID}, ConversationID: conversation.ID,
		UserID: userID, LastReadSeq: read.LastReadSeq, UnreadCount: &unread,
	}}
	if conversation.Kind != KindDirect {
		return notes, nil
	}

	recipients, err := m.members.recipients(ctx, conversation.ID, userID)
	if err != nil {
		return nil, err
	}
	recipients = slices.DeleteFunc(recipients, func(id uuid.UUID) bool { return id == userID })
	return append(notes, notification{
		Event: eventReadUpdated, Recipients: recipients, ConversationID: conversation.ID,
		UserID: userID, LastReadSeq: read.LastReadSeq,
	}), nil
}

func (m Module) unread(ctx context.Context, viewerID uuid.UUID) (int, int, error) {
	return m.members.unreadTotals(ctx, viewerID)
}
