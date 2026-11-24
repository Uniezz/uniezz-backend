package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strconv"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

var errReplayed = errors.New("message already stored for this idempotency key")

type messageQuery struct {
	ViewerID       uuid.UUID
	ConversationID uuid.UUID
	Cursor         string
	ChangedAfter   string
	Limit          int
}

type sendChange struct {
	ConversationID uuid.UUID
	Body           string
	AttachmentIDs  []uuid.UUID
	Key            string
}

type sendInput struct {
	ConversationID uuid.UUID
	Body           string
	MediaIDs       []uuid.UUID
	Key            uuid.UUID
}

func (c sendChange) clean() (sendInput, error) {
	if len(c.AttachmentIDs) > maxAttachments || hasDuplicates(c.AttachmentIDs) {
		return sendInput{}, ErrInvalidInput
	}
	body, err := cleanBody(c.Body, len(c.AttachmentIDs))
	if err != nil {
		return sendInput{}, err
	}
	key, err := parseKey(c.Key)
	if err != nil || key == nil {
		return sendInput{}, ErrInvalidInput
	}
	mediaIDs := c.AttachmentIDs
	if mediaIDs == nil {
		mediaIDs = []uuid.UUID{}
	}
	return sendInput{ConversationID: c.ConversationID, Body: body, MediaIDs: mediaIDs, Key: *key}, nil
}

func hasDuplicates(ids []uuid.UUID) bool {
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

func sortedIDs(ids []uuid.UUID) []uuid.UUID {
	sorted := slices.Clone(ids)
	slices.SortFunc(sorted, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	return sorted
}

func mediaError(err error) error {
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
		return errors.Join(ErrInvalidMedia, err)
	}
	return err
}

func (m Module) checkUploads(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, id := range ids {
		g.Go(func() error {
			return mediaError(m.media.CheckUpload(ctx, id, userID, media.KindChat))
		})
	}
	return g.Wait()
}

func (m Module) sendMessage(ctx context.Context, senderID uuid.UUID, c sendChange) (*Message, bool, error) {
	in, err := c.clean()
	if err != nil {
		return nil, false, err
	}

	message, err := m.replayMessage(ctx, senderID, in)
	if !errors.Is(err, ErrNotFound) {
		return message, err == nil, err
	}
	if err := m.checkUploads(ctx, senderID, in.MediaIDs); err != nil {
		return nil, false, err
	}

	var created *messageRow
	var notes []notification
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.sends.Allow(ctx, senderID.String()); err != nil {
			if errors.Is(err, ratelimit.ErrLimited) {
				return errors.Join(ErrRateLimited, err)
			}
			return err
		}
		locked, err := m.conversations.bumpMessageAsMember(ctx, in.ConversationID, senderID)
		if err != nil {
			return err
		}
		a, err := m.conversations.access(ctx, senderID, in.ConversationID)
		if err != nil {
			return err
		}
		if err := a.viewable(); err != nil {
			return err
		}
		if _, err := m.messages.byClient(ctx, senderID, in.Key); err == nil {
			return errReplayed
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := m.checkRequest(ctx, senderID, a, locked.LastSeq, in); err != nil {
			return err
		}

		created, err = m.insertText(ctx, senderID, in, locked)
		if err != nil {
			return err
		}
		recipients, err := m.members.recipients(ctx, in.ConversationID, senderID)
		if err != nil {
			return err
		}
		notes = []notification{{Event: eventMessageCreated, Recipients: recipients, ConversationID: in.ConversationID, ChangeSeq: created.ChangeSeq, Message: created}}
		return nil
	})
	if errors.Is(err, errReplayed) || errors.Is(err, errClientTaken) {
		message, err := m.replayMessage(ctx, senderID, in)
		return message, err == nil, err
	}
	if err != nil {
		return nil, false, err
	}

	views, err := m.messageViews(ctx, []messageRow{*created})
	if err != nil {
		return nil, false, err
	}
	notes[0].Hydrated = &views[0]
	m.publish(ctx, notes)
	return &views[0], false, nil
}

func (m Module) insertText(ctx context.Context, senderID uuid.UUID, in sendInput, conversation *lockedRow) (*messageRow, error) {
	var body *string
	if in.Body != "" {
		body = &in.Body
	}
	created, err := m.messages.insert(ctx, newMessage{
		ConversationID: in.ConversationID,
		Seq:            conversation.LastSeq,
		ChangeSeq:      conversation.ChangeSeq,
		SenderID:       &senderID,
		Kind:           MessageText,
		Body:           body,
		Payload:        []byte("{}"),
		ClientID:       &in.Key,
	})
	if err != nil {
		return nil, err
	}

	for _, id := range sortedIDs(in.MediaIDs) {
		if err := m.media.MarkReady(ctx, id, senderID, media.KindChat); err != nil {
			return nil, mediaError(err)
		}
	}
	for i, id := range in.MediaIDs {
		if err := m.messages.attach(ctx, created.ID, i, id); err != nil {
			return nil, err
		}
	}
	if err := m.members.countUnread(ctx, in.ConversationID, senderID); err != nil {
		return nil, err
	}
	if err := m.members.markReadThrough(ctx, in.ConversationID, senderID, created.Seq); err != nil {
		return nil, err
	}
	return created, nil
}

func (m Module) replayMessage(ctx context.Context, senderID uuid.UUID, in sendInput) (*Message, error) {
	row, err := m.messages.byClient(ctx, senderID, in.Key)
	if err != nil {
		return nil, err
	}
	if row.ConversationID != in.ConversationID || row.Kind != MessageText {
		return nil, ErrKeyReused
	}
	if row.DeletedAt != nil {
		return nil, ErrResourceDeleted
	}

	mediaIDs, err := m.messages.mediaIDs(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	body := ""
	if row.Body != nil {
		body = *row.Body
	}
	if body != in.Body || !slices.Equal(mediaIDs, in.MediaIDs) {
		return nil, ErrKeyReused
	}

	views, err := m.messageViews(ctx, []messageRow{*row})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

func (m Module) listMessages(ctx context.Context, q messageQuery) ([]Message, *string, error) {
	a, err := m.conversations.access(ctx, q.ViewerID, q.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	if err := a.viewable(); err != nil {
		return nil, nil, err
	}

	filter := messageFilter{ConversationID: q.ConversationID, ViewerID: q.ViewerID, Limit: q.Limit + 1}
	catchUp := q.ChangedAfter != ""
	switch {
	case catchUp:
		after, err := strconv.ParseInt(q.ChangedAfter, 10, 64)
		if err != nil {
			return nil, nil, ErrInvalidInput
		}
		if q.Cursor != "" {
			var cursor changeCursor
			if err := httpapi.DecodeCursor(q.Cursor, &cursor); err != nil {
				return nil, nil, err
			}
			after = cursor.ChangeSeq
		}
		filter.AfterChange = &after
	case q.Cursor != "":
		var cursor messageCursor
		if err := httpapi.DecodeCursor(q.Cursor, &cursor); err != nil {
			return nil, nil, err
		}
		filter.BeforeSeq = &cursor.Seq
	}

	rows, err := m.messages.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last messageRow) any {
		if catchUp {
			return changeCursor{ChangeSeq: last.ChangeSeq}
		}
		return messageCursor{Seq: last.Seq}
	})
	if err != nil {
		return nil, nil, err
	}

	items, err := m.messageViews(ctx, rows)
	if err != nil {
		return nil, nil, err
	}
	return items, next, nil
}

func (m Module) getMessage(ctx context.Context, viewerID, id uuid.UUID) (*Message, error) {
	row, err := m.messages.getVisible(ctx, viewerID, id)
	if err != nil {
		return nil, err
	}
	views, err := m.messageViews(ctx, []messageRow{*row})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

func (m Module) deleteMessage(ctx context.Context, userID, id uuid.UUID) error {
	row, err := m.messages.get(ctx, id)
	if err != nil {
		return err
	}

	var released []uuid.UUID
	var notes []notification
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.conversations.lockAsMember(ctx, row.ConversationID, userID); err != nil {
			return err
		}
		a, err := m.conversations.access(ctx, userID, row.ConversationID)
		if err != nil {
			return err
		}
		if err := a.viewable(); err != nil {
			return err
		}
		released, notes, err = m.tombstone(ctx, id, &userID)
		return err
	})
	if err != nil {
		return err
	}

	m.releaseMedia(ctx, released)
	m.publish(ctx, notes)
	return nil
}

func (m Module) tombstone(ctx context.Context, id uuid.UUID, owner *uuid.UUID) ([]uuid.UUID, []notification, error) {
	row, err := m.messages.get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if owner != nil && (row.SenderID == nil || *row.SenderID != *owner || row.Kind != MessageText) {
		return nil, nil, ErrForbidden
	}
	if row.DeletedAt != nil {
		return nil, nil, nil
	}

	changeSeq, err := m.conversations.bumpChange(ctx, row.ConversationID)
	if err != nil {
		return nil, nil, err
	}
	if err := m.messages.tombstone(ctx, id, changeSeq); err != nil {
		return nil, nil, err
	}
	released, err := m.messages.detachMedia(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if len(released) > 0 {
		if err := m.media.MarkDeletingMany(ctx, released); err != nil {
			return nil, nil, err
		}
	}

	updated, err := m.messages.get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	actorID := uuid.Nil
	if row.SenderID != nil {
		actorID = *row.SenderID
	}
	recipients, err := m.members.recipients(ctx, row.ConversationID, actorID)
	if err != nil {
		return nil, nil, err
	}
	note := notification{Event: eventMessageUpdated, Recipients: recipients, ConversationID: row.ConversationID, ChangeSeq: changeSeq, Message: updated}
	return released, []notification{note}, nil
}

func (m Module) releaseMedia(ctx context.Context, ids []uuid.UUID) {
	ctx = context.WithoutCancel(ctx)
	for _, id := range ids {
		if err := m.media.Delete(ctx, id); err != nil {
			slog.WarnContext(ctx, "delete chat media", slog.Any("error", err))
		}
	}
}

func (m Module) messageViews(ctx context.Context, rows []messageRow) ([]Message, error) {
	items := make([]Message, 0, len(rows))
	if len(rows) == 0 {
		return items, nil
	}

	senderIDs := make([]uuid.UUID, 0, len(rows))
	messageIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		messageIDs = append(messageIDs, row.ID)
		if row.SenderID != nil {
			senderIDs = append(senderIDs, *row.SenderID)
		}
	}
	students, err := m.studentMap(ctx, senderIDs)
	if err != nil {
		return nil, err
	}
	attachmentRows, err := m.messages.attachments(ctx, messageIDs)
	if err != nil {
		return nil, err
	}

	expiresAt := m.now().Add(media.PrivateURLTTL)
	attachments := make(map[uuid.UUID][]Attachment, len(rows))
	for _, a := range attachmentRows {
		url := m.mediaURL(ctx, &a.Ref)
		if url == nil {
			continue
		}
		attachments[a.MessageID] = append(attachments[a.MessageID], Attachment{MediaID: a.MediaID, URL: *url, ExpiresAt: expiresAt})
	}

	for _, row := range rows {
		payload := map[string]any{}
		if err := json.Unmarshal(row.Payload, &payload); err != nil || payload == nil {
			payload = map[string]any{}
		}
		items = append(items, Message{
			ID:             row.ID,
			ConversationID: row.ConversationID,
			Seq:            row.Seq,
			ChangeSeq:      row.ChangeSeq,
			Kind:           row.Kind,
			Sender:         studentOf(students, row.SenderID),
			Body:           row.Body,
			Payload:        payload,
			ClientID:       row.ClientID,
			Attachments:    nonNilAttachments(attachments[row.ID]),
			CreatedAt:      row.CreatedAt,
			DeletedAt:      row.DeletedAt,
		})
	}
	return items, nil
}

func nonNilAttachments(a []Attachment) []Attachment {
	if a == nil {
		return []Attachment{}
	}
	return a
}
