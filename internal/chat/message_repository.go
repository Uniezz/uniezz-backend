package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	uniqueViolation  = "23505"
	clientConstraint = "uq_chat_messages_client"
)

var errClientTaken = errors.New("client id already used by another message")

type messageRow struct {
	ID             uuid.UUID
	ConversationID uuid.UUID
	Seq            int64
	ChangeSeq      int64
	SenderID       *uuid.UUID
	Kind           MessageKind
	Body           *string
	Payload        []byte
	ClientID       *uuid.UUID
	CreatedAt      time.Time
	DeletedAt      *time.Time
}

type newMessage struct {
	ConversationID uuid.UUID
	Seq            int64
	ChangeSeq      int64
	SenderID       *uuid.UUID
	Kind           MessageKind
	Body           *string
	Payload        []byte
	ClientID       *uuid.UUID
}

type attachmentRow struct {
	MessageID uuid.UUID
	MediaID   uuid.UUID
	Ref       media.Ref
}

type messageCursor struct {
	Seq   int64 `json:"s"`
	Newer bool  `json:"n,omitempty"`
}

type changeCursor struct {
	ChangeSeq int64 `json:"c"`
}

type messageFilter struct {
	ConversationID uuid.UUID
	ViewerID       uuid.UUID
	BeforeSeq      *int64
	AfterSeq       *int64
	AfterChange    *int64
	Limit          int
}

type searchFilter struct {
	ConversationID uuid.UUID
	ViewerID       uuid.UUID
	Words          []string
	BeforeSeq      *int64
	Limit          int
}

type messageRepository struct {
	db database.DB
}

func newMessageRepository(db database.DB) *messageRepository {
	return &messageRepository{db: db}
}

func messageVisible(viewer, sender string) string {
	return "(" + sender + " IS NULL OR (" +
		"NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = " + viewer + " AND b.other_id = " + sender + ")" +
		" AND NOT EXISTS (SELECT 1 FROM banned_users x WHERE x.id = " + sender + ")))"
}

const messageColumns = "msg.id, msg.conversation_id, msg.seq, msg.change_seq, msg.sender_id, msg.kind, msg.body, msg.payload, msg.client_id, msg.created_at, msg.deleted_at"

func scanMessage(row pgx.Row) (*messageRow, error) {
	var m messageRow
	err := row.Scan(&m.ID, &m.ConversationID, &m.Seq, &m.ChangeSeq, &m.SenderID, &m.Kind, &m.Body, &m.Payload, &m.ClientID, &m.CreatedAt, &m.DeletedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (r *messageRepository) scanMessages(ctx context.Context, what, query string, args ...any) ([]messageRow, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()

	var messages []messageRow
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		messages = append(messages, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return messages, nil
}

func (r *messageRepository) insert(ctx context.Context, n newMessage) (*messageRow, error) {
	const query = `
		INSERT INTO chat_messages AS msg (conversation_id, seq, change_seq, sender_id, kind, body, payload, client_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING ` + messageColumns
	m, err := scanMessage(r.db.Conn(ctx).QueryRow(ctx, query,
		n.ConversationID, n.Seq, n.ChangeSeq, n.SenderID, n.Kind, n.Body, n.Payload, n.ClientID))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == clientConstraint {
		return nil, errClientTaken
	}
	if err != nil {
		return nil, fmt.Errorf("insert message: %w", err)
	}
	return m, nil
}

func (r *messageRepository) attach(ctx context.Context, messageID uuid.UUID, position int, mediaID uuid.UUID) error {
	const query = `INSERT INTO chat_message_media (message_id, position, media_id) VALUES ($1, $2, $3)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, messageID, position, mediaID); err != nil {
		return fmt.Errorf("attach message media: %w", err)
	}
	return nil
}

func (r *messageRepository) get(ctx context.Context, id uuid.UUID) (*messageRow, error) {
	const query = `SELECT ` + messageColumns + ` FROM chat_messages msg WHERE msg.id = $1`
	m, err := scanMessage(r.db.Conn(ctx).QueryRow(ctx, query, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	return m, nil
}

func (r *messageRepository) getVisible(ctx context.Context, viewerID, id uuid.UUID) (*messageRow, error) {
	args := database.Args{viewerID, id}
	query := `
		SELECT ` + messageColumns + `
		FROM chat_messages msg
		JOIN chat_conversations c ON c.id = msg.conversation_id
		JOIN chat_members m ON m.conversation_id = msg.conversation_id AND m.user_id = $1
		WHERE msg.id = $2
		  AND ` + directVisible(args.Add, "$1", false) + `
		  AND ` + messageVisible("$1", "msg.sender_id")
	m, err := scanMessage(r.db.Conn(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get visible message: %w", err)
	}
	return m, nil
}

func (r *messageRepository) byClient(ctx context.Context, senderID, clientID uuid.UUID) (*messageRow, error) {
	const query = `SELECT ` + messageColumns + ` FROM chat_messages msg WHERE msg.sender_id = $1 AND msg.client_id = $2`
	m, err := scanMessage(r.db.Conn(ctx).QueryRow(ctx, query, senderID, clientID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get message by client id: %w", err)
	}
	return m, nil
}

func (r *messageRepository) list(ctx context.Context, f messageFilter) ([]messageRow, error) {
	args := database.Args{f.ConversationID, f.ViewerID}
	arg := args.Add

	where := "msg.conversation_id = $1 AND " + messageVisible("$2", "msg.sender_id")
	order := "msg.seq DESC"
	if f.BeforeSeq != nil {
		where += " AND msg.seq < " + arg(*f.BeforeSeq)
	}
	if f.AfterSeq != nil {
		where += " AND msg.seq > " + arg(*f.AfterSeq)
		order = "msg.seq ASC"
	}
	if f.AfterChange != nil {
		where += " AND msg.change_seq > " + arg(*f.AfterChange)
		order = "msg.change_seq ASC"
	}
	query := `SELECT ` + messageColumns + ` FROM chat_messages msg WHERE ` + where + ` ORDER BY ` + order + ` LIMIT ` + arg(f.Limit)
	return r.scanMessages(ctx, "list messages", query, args...)
}

func (r *messageRepository) search(ctx context.Context, f searchFilter) ([]messageRow, error) {
	args := database.Args{f.ConversationID, f.ViewerID}
	arg := args.Add

	where := []string{
		"msg.conversation_id = $1",
		"msg.body IS NOT NULL",
		"msg.deleted_at IS NULL",
		messageVisible("$2", "msg.sender_id"),
	}
	for _, word := range f.Words {
		where = append(where, "public.search_text(msg.body) LIKE "+database.SearchPattern(arg(word)))
	}
	if f.BeforeSeq != nil {
		where = append(where, "msg.seq < "+arg(*f.BeforeSeq))
	}
	query := `SELECT ` + messageColumns + ` FROM chat_messages msg WHERE ` + strings.Join(where, " AND ") +
		` ORDER BY msg.seq DESC LIMIT ` + arg(f.Limit)
	return r.scanMessages(ctx, "search messages", query, args...)
}

func (r *messageRepository) lastVisible(ctx context.Context, viewerID uuid.UUID, conversationIDs []uuid.UUID) ([]messageRow, error) {
	query := `
		SELECT DISTINCT ON (msg.conversation_id) ` + messageColumns + `
		FROM chat_messages msg
		WHERE msg.conversation_id = ANY($2) AND ` + messageVisible("$1", "msg.sender_id") + `
		ORDER BY msg.conversation_id, msg.seq DESC`
	return r.scanMessages(ctx, "list last messages", query, viewerID, conversationIDs)
}

func (r *messageRepository) attachments(ctx context.Context, messageIDs []uuid.UUID) ([]attachmentRow, error) {
	const query = `
		SELECT mm.message_id, mm.media_id, m.kind, m.object_key
		FROM chat_message_media mm
		JOIN ready_media m ON m.id = mm.media_id
		WHERE mm.message_id = ANY($1)
		ORDER BY mm.message_id, mm.position`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("list message media: %w", err)
	}
	defer rows.Close()

	var attachments []attachmentRow
	for rows.Next() {
		var a attachmentRow
		if err := rows.Scan(&a.MessageID, &a.MediaID, &a.Ref.Kind, &a.Ref.ObjectKey); err != nil {
			return nil, fmt.Errorf("scan message media: %w", err)
		}
		attachments = append(attachments, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list message media: %w", err)
	}
	return attachments, nil
}

func (r *messageRepository) mediaIDs(ctx context.Context, messageID uuid.UUID) ([]uuid.UUID, error) {
	const query = `SELECT media_id FROM chat_message_media WHERE message_id = $1 ORDER BY position`
	return scanUUIDs(ctx, r.db, "list message media ids", query, messageID)
}

func (r *messageRepository) conversationMediaIDs(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error) {
	const query = `
		SELECT mm.media_id
		FROM chat_message_media mm
		JOIN chat_messages msg ON msg.id = mm.message_id
		WHERE msg.conversation_id = $1
		ORDER BY mm.media_id`
	return scanUUIDs(ctx, r.db, "list conversation media ids", query, conversationID)
}

func (r *messageRepository) detachMedia(ctx context.Context, messageID uuid.UUID) ([]uuid.UUID, error) {
	const query = `DELETE FROM chat_message_media WHERE message_id = $1 RETURNING media_id`
	return scanUUIDs(ctx, r.db, "detach message media", query, messageID)
}

func (r *messageRepository) tombstone(ctx context.Context, id uuid.UUID, changeSeq int64) error {
	const query = `
		UPDATE chat_messages
		SET deleted_at = NOW(), body = NULL, payload = '{}', change_seq = $2
		WHERE id = $1 AND deleted_at IS NULL`
	result, err := r.db.Conn(ctx).Exec(ctx, query, id, changeSeq)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *messageRepository) setChange(ctx context.Context, id uuid.UUID, changeSeq int64) error {
	const query = `UPDATE chat_messages SET change_seq = $2 WHERE id = $1`
	result, err := r.db.Conn(ctx).Exec(ctx, query, id, changeSeq)
	if err != nil {
		return fmt.Errorf("touch message: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *messageRepository) liveIDsBySender(ctx context.Context, senderID uuid.UUID) ([]uuid.UUID, error) {
	const query = `
		SELECT id FROM chat_messages
		WHERE sender_id = $1 AND kind <> $2 AND deleted_at IS NULL
		ORDER BY id`
	return scanUUIDs(ctx, r.db, "list sender messages", query, senderID, MessageSystem)
}

func scanUUIDs(ctx context.Context, db database.DB, what, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: %w", what, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return ids, nil
}
