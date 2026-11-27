package chat

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/google/uuid"
)

type conversationQuery struct {
	ViewerID uuid.UUID
	Requests bool
	Cursor   string
	Limit    int
}

type groupChange struct {
	Name      string
	MemberIDs []uuid.UUID
	Key       string
}

type renameChange struct {
	ConversationID uuid.UUID
	Name           string
}

func (m Module) getConversation(ctx context.Context, viewerID, id uuid.UUID) (*Conversation, error) {
	row, err := m.conversations.get(ctx, viewerID, id)
	if err != nil {
		return nil, err
	}
	items, err := m.conversationViews(ctx, viewerID, []conversationRow{*row})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (m Module) listConversations(ctx context.Context, q conversationQuery) ([]Conversation, *string, error) {
	filter := conversationFilter{ViewerID: q.ViewerID, Requests: q.Requests, Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after conversationCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.conversations.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last conversationRow) any {
		return conversationCursor{SortAt: last.SortAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	items, err := m.conversationViews(ctx, q.ViewerID, rows)
	if err != nil {
		return nil, nil, err
	}
	return items, next, nil
}

func (m Module) conversationViews(ctx context.Context, viewerID uuid.UUID, rows []conversationRow) ([]Conversation, error) {
	conversationIDs := make([]uuid.UUID, 0, len(rows))
	peerIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		conversationIDs = append(conversationIDs, row.ID)
		if row.PeerID != nil {
			peerIDs = append(peerIDs, *row.PeerID)
		}
	}

	lastRows, err := m.messages.lastVisible(ctx, viewerID, conversationIDs)
	if err != nil {
		return nil, err
	}
	lastMessages, err := m.messageViews(ctx, lastRows)
	if err != nil {
		return nil, err
	}
	lastByConversation := make(map[uuid.UUID]Message, len(lastMessages))
	for _, message := range lastMessages {
		lastByConversation[message.ConversationID] = message
	}
	peers, err := m.studentMap(ctx, peerIDs)
	if err != nil {
		return nil, err
	}

	items := make([]Conversation, 0, len(rows))
	for _, row := range rows {
		item := Conversation{
			ID:          row.ID,
			Kind:        row.Kind,
			Name:        row.Name,
			LastSeq:     row.LastSeq,
			ChangeSeq:   row.ChangeSeq,
			LastReadSeq: row.LastReadSeq,
			UnreadCount: row.UnreadCount,
			CreatedAt:   row.CreatedAt,
		}
		if last, ok := lastByConversation[row.ID]; ok {
			item.LastMessage = &last
		}
		if row.Kind == KindDirect {
			peer := studentOf(peers, row.PeerID)
			item.Peer = &peer
			status := row.Status
			if row.Connected || status == StatusApproved {
				status = StatusApproved
			} else {
				status = StatusPending
			}
			incoming := row.StartedBy != nil && *row.StartedBy != viewerID
			item.Origin, item.Status, item.Incoming = row.Origin, &status, &incoming
		} else {
			count, role := row.MemberCount, row.MyRole
			item.MemberCount = &count
			item.MyRole = &role
		}
		items = append(items, item)
	}
	return items, nil
}

func (m Module) openDirect(ctx context.Context, viewerID, otherID uuid.UUID) (*Conversation, bool, error) {
	if viewerID == otherID {
		return nil, false, ErrInvalidInput
	}

	var id uuid.UUID
	var created bool
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		connected, err := m.members.connected(ctx, viewerID, []uuid.UUID{otherID})
		if err != nil {
			return err
		}
		if len(connected) > 0 {
			id, created, err = m.conversations.openDirect(ctx, viewerID, otherID, OriginMatch, StatusApproved)
			if err != nil {
				return err
			}
			if !created {
				return m.conversations.approve(ctx, id)
			}
			return m.members.insert(ctx, id, []uuid.UUID{viewerID, otherID}, RoleMember, 0)
		}

		existingID, startedBy, err := m.conversations.findDirect(ctx, viewerID, otherID)
		if err == nil {
			id = existingID
			if startedBy != viewerID {
				return m.conversations.approve(ctx, id)
			}
			return nil
		}
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		allowed, err := m.requests.canStart(ctx, requestFilter{SenderID: viewerID, RecipientID: otherID, AdultAge: adultAge})
		if err != nil {
			return err
		}
		if !allowed {
			return ErrNotFound
		}
		return ErrRequestRequired
	})
	if err != nil {
		return nil, false, err
	}

	conversation, err := m.getConversation(ctx, viewerID, id)
	if err != nil {
		return nil, false, err
	}
	return conversation, created, nil
}

func cleanMemberIDs(creatorID uuid.UUID, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) < 1 || len(ids) > maxGroupMembers-1 {
		return nil, ErrInvalidInput
	}
	unique := slices.Clone(ids)
	slices.SortFunc(unique, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	unique = slices.Compact(unique)
	if len(unique) != len(ids) || slices.Contains(unique, creatorID) {
		return nil, ErrInvalidInput
	}
	return unique, nil
}

func parseKey(key string) (*uuid.UUID, error) {
	if key == "" {
		return nil, nil
	}
	id, err := uuid.Parse(key)
	if err != nil {
		return nil, ErrInvalidInput
	}
	return &id, nil
}

func (m Module) createGroup(ctx context.Context, creatorID uuid.UUID, c groupChange) (*Conversation, bool, error) {
	name, err := cleanName(c.Name)
	if err != nil {
		return nil, false, err
	}
	memberIDs, err := cleanMemberIDs(creatorID, c.MemberIDs)
	if err != nil {
		return nil, false, err
	}
	key, err := parseKey(c.Key)
	if err != nil {
		return nil, false, err
	}

	if key != nil {
		conversation, err := m.replayGroup(ctx, creatorID, *key, name, memberIDs)
		if !errors.Is(err, ErrNotFound) {
			return conversation, err == nil, err
		}
	}

	connected, err := m.members.connected(ctx, creatorID, memberIDs)
	if err != nil {
		return nil, false, err
	}
	if len(connected) != len(memberIDs) {
		return nil, false, ErrInvalidMember
	}

	var conversationID uuid.UUID
	var notes []notification
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.groups.Allow(ctx, creatorID.String()); err != nil {
			if errors.Is(err, ratelimit.ErrLimited) {
				return errors.Join(ErrRateLimited, err)
			}
			return err
		}
		var err error
		conversationID, err = m.conversations.createGroup(ctx, name)
		if err != nil {
			return err
		}
		if err := m.members.insert(ctx, conversationID, []uuid.UUID{creatorID}, RoleOwner, 0); err != nil {
			return err
		}
		created, err := m.writeSystemMessage(ctx, conversationID, creatorID, systemGroupCreated, groupParams(name, memberIDs), key)
		if err != nil {
			return err
		}
		if err := m.members.insert(ctx, conversationID, memberIDs, RoleMember, created.Seq); err != nil {
			return err
		}
		notes, err = m.membershipNotifications(ctx, conversationID, creatorID, created)
		return err
	})
	if errors.Is(err, errClientTaken) {
		conversation, err := m.replayGroup(ctx, creatorID, *key, name, memberIDs)
		return conversation, err == nil, err
	}
	if err != nil {
		return nil, false, err
	}

	m.publish(ctx, notes)
	conversation, err := m.getConversation(ctx, creatorID, conversationID)
	if err != nil {
		return nil, false, err
	}
	return conversation, false, nil
}

func groupParams(name string, memberIDs []uuid.UUID) map[string]any {
	ids := make([]string, 0, len(memberIDs))
	for _, id := range memberIDs {
		ids = append(ids, id.String())
	}
	return map[string]any{"name": name, "memberIds": ids}
}

func (m Module) replayGroup(ctx context.Context, creatorID, key uuid.UUID, name string, memberIDs []uuid.UUID) (*Conversation, error) {
	row, err := m.messages.byClient(ctx, creatorID, key)
	if err != nil {
		return nil, err
	}

	var payload systemPayload
	if row.Kind != MessageSystem || json.Unmarshal(row.Payload, &payload) != nil || payload.Event != systemGroupCreated {
		return nil, ErrKeyReused
	}
	want, _ := json.Marshal(groupParams(name, memberIDs))
	got, _ := json.Marshal(payload.Params)
	if string(want) != string(got) {
		return nil, ErrKeyReused
	}

	conversation, err := m.getConversation(ctx, creatorID, row.ConversationID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrResourceDeleted
	}
	return conversation, err
}

func (m Module) renameConversation(ctx context.Context, userID uuid.UUID, c renameChange) (*Conversation, error) {
	name, err := cleanName(c.Name)
	if err != nil {
		return nil, err
	}

	var notes []notification
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		locked, err := m.conversations.lockAsMember(ctx, c.ConversationID, userID)
		if err != nil {
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
		if locked.Name != nil && *locked.Name == name {
			return nil
		}

		if err := m.conversations.rename(ctx, c.ConversationID, name); err != nil {
			return err
		}
		renamed, err := m.writeSystemMessage(ctx, c.ConversationID, userID, systemRenamed, map[string]any{"name": name}, nil)
		if err != nil {
			return err
		}
		notes, err = m.membershipNotifications(ctx, c.ConversationID, userID, renamed)
		return err
	})
	if err != nil {
		return nil, err
	}

	m.publish(ctx, notes)
	return m.getConversation(ctx, userID, c.ConversationID)
}
