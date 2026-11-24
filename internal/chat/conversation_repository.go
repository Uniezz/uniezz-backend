package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type accessRow struct {
	Kind          Kind
	Role          Role
	DirectVisible bool
	Status        Status
	StartedBy     *uuid.UUID
	Connected     bool
}

type conversationRow struct {
	ID            uuid.UUID
	Kind          Kind
	Name          *string
	PeerID        *uuid.UUID
	LastSeq       int64
	ChangeSeq     int64
	LastMessageAt *time.Time
	CreatedAt     time.Time
	MyRole        Role
	LastReadSeq   int64
	UnreadCount   int
	MemberCount   int
	SortAt        time.Time
	Status        Status
	Origin        *Origin
	StartedBy     *uuid.UUID
	Connected     bool
}

type conversationCursor struct {
	SortAt time.Time `json:"t"`
	ID     uuid.UUID `json:"i"`
}

type conversationFilter struct {
	ViewerID uuid.UUID
	Requests bool
	After    *conversationCursor
	Limit    int
}

type lockedRow struct {
	ID        uuid.UUID
	Kind      Kind
	Name      *string
	LastSeq   int64
	ChangeSeq int64
	DirectLow *uuid.UUID
}

type conversationRepository struct {
	db database.DB
}

func newConversationRepository(db database.DB) *conversationRepository {
	return &conversationRepository{db: db}
}

func peerOf(viewer string) string {
	return "(CASE WHEN c.direct_user_low = " + viewer + " THEN c.direct_user_high ELSE c.direct_user_low END)"
}

func pairConnected(viewer string) string {
	return "EXISTS (SELECT 1 FROM connected_pairs p WHERE p.user_id = " + viewer + " AND p.other_id = " + peerOf(viewer) + ")"
}

func pairOpen(viewer string) string {
	return "NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = " + viewer + " AND b.other_id = " + peerOf(viewer) + ")" +
		" AND NOT EXISTS (SELECT 1 FROM banned_users x WHERE x.id = " + peerOf(viewer) + ")"
}

func directVisible(arg func(any) string, viewer string, inbox bool) string {
	own := "c.started_by = " + viewer
	if inbox {
		own = "(" + own + " AND c.last_seq > 0)"
	}
	allowed := "c.status = " + arg(StatusApproved) + " OR " + own + " OR " + pairConnected(viewer)
	if !inbox {
		allowed += " OR (c.status = " + arg(StatusPending) + " AND c.last_seq > 0)"
	}
	return "(c.direct_user_low IS NULL OR (" + pairOpen(viewer) + " AND (" + allowed + ")))"
}

func incomingRequest(arg func(any) string, viewer string) string {
	return "(c.kind = " + arg(KindDirect) + " AND c.status = " + arg(StatusPending) + " AND c.started_by <> " + viewer +
		" AND c.last_seq > 0 AND " + pairOpen(viewer) + " AND NOT " + pairConnected(viewer) + ")"
}

const conversationSelect = `
	SELECT c.id, c.kind, c.name,
		CASE WHEN c.direct_user_low = $1 THEN c.direct_user_high ELSE c.direct_user_low END,
		c.last_seq, c.change_seq, c.last_message_at, c.created_at,
		m.role, m.last_read_seq, m.unread_count,
		CASE WHEN c.kind = $2 THEN (SELECT COUNT(*) FROM chat_members x WHERE x.conversation_id = c.id) ELSE 0 END,
		COALESCE(c.last_message_at, c.created_at),
		c.status, c.origin, c.started_by,
		c.direct_user_low IS NOT NULL AND EXISTS (SELECT 1 FROM connected_pairs p WHERE p.user_id = $1
			AND p.other_id = CASE WHEN c.direct_user_low = $1 THEN c.direct_user_high ELSE c.direct_user_low END)
	FROM chat_conversations c
	JOIN chat_members m ON m.conversation_id = c.id AND m.user_id = $1`

func scanConversation(row pgx.Row) (*conversationRow, error) {
	var c conversationRow
	err := row.Scan(&c.ID, &c.Kind, &c.Name, &c.PeerID, &c.LastSeq, &c.ChangeSeq, &c.LastMessageAt, &c.CreatedAt,
		&c.MyRole, &c.LastReadSeq, &c.UnreadCount, &c.MemberCount, &c.SortAt,
		&c.Status, &c.Origin, &c.StartedBy, &c.Connected)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *conversationRepository) get(ctx context.Context, viewerID, id uuid.UUID) (*conversationRow, error) {
	args := database.Args{viewerID, KindGroup}
	arg := args.Add
	query := conversationSelect + `
	WHERE c.id = ` + arg(id) + ` AND ` + directVisible(arg, "$1", false)

	c, err := scanConversation(r.db.Conn(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	return c, nil
}

func (r *conversationRepository) list(ctx context.Context, f conversationFilter) ([]conversationRow, error) {
	args := database.Args{f.ViewerID, KindGroup}
	arg := args.Add

	var where string
	if f.Requests {
		where = incomingRequest(arg, "$1")
	} else {
		where = directVisible(arg, "$1", true)
	}
	if f.After != nil {
		where += " AND (COALESCE(c.last_message_at, c.created_at), c.id) < (" + arg(f.After.SortAt) + ", " + arg(f.After.ID) + ")"
	}
	query := conversationSelect + `
	WHERE ` + where + `
	ORDER BY COALESCE(c.last_message_at, c.created_at) DESC, c.id DESC
	LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	var conversations []conversationRow
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		conversations = append(conversations, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	return conversations, nil
}

func (r *conversationRepository) access(ctx context.Context, viewerID, id uuid.UUID) (*accessRow, error) {
	args := database.Args{viewerID, id}
	arg := args.Add
	query := `
		SELECT c.kind, m.role, ` + directVisible(arg, "$1", false) + `, c.status, c.started_by,
			c.direct_user_low IS NOT NULL AND ` + pairConnected("$1") + `
		FROM chat_members m
		JOIN chat_conversations c ON c.id = m.conversation_id
		WHERE m.conversation_id = $2 AND m.user_id = $1`

	var a accessRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, args...).Scan(&a.Kind, &a.Role, &a.DirectVisible, &a.Status, &a.StartedBy, &a.Connected)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get conversation access: %w", err)
	}
	return &a, nil
}

func (r *conversationRepository) openDirect(ctx context.Context, userID, otherID uuid.UUID, origin Origin, status Status) (uuid.UUID, bool, error) {
	const insert = `
		INSERT INTO chat_conversations (kind, direct_user_low, direct_user_high, origin, status, started_by)
		VALUES ($3, LEAST($1, $2)::uuid, GREATEST($1, $2)::uuid, $4, $5, $1::uuid)
		ON CONFLICT (direct_user_low, direct_user_high) DO NOTHING
		RETURNING id`
	var id uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, insert, userID, otherID, KindDirect, origin, status).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, fmt.Errorf("create direct conversation: %w", err)
	}

	const find = `
		SELECT id FROM chat_conversations
		WHERE direct_user_low = LEAST($1, $2)::uuid AND direct_user_high = GREATEST($1, $2)::uuid`
	if err := r.db.Conn(ctx).QueryRow(ctx, find, userID, otherID).Scan(&id); err != nil {
		return uuid.Nil, false, fmt.Errorf("find direct conversation: %w", err)
	}
	return id, false, nil
}

func (r *conversationRepository) findDirect(ctx context.Context, userID, otherID uuid.UUID) (uuid.UUID, uuid.UUID, error) {
	const query = `
		SELECT id, started_by FROM chat_conversations
		WHERE direct_user_low = LEAST($1, $2)::uuid AND direct_user_high = GREATEST($1, $2)::uuid`
	var id, startedBy uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID, otherID).Scan(&id, &startedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("find direct conversation: %w", err)
	}
	return id, startedBy, nil
}

func (r *conversationRepository) approve(ctx context.Context, id uuid.UUID) error {
	const query = `UPDATE chat_conversations SET status = $2 WHERE id = $1 AND status <> $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, StatusApproved); err != nil {
		return fmt.Errorf("approve conversation: %w", err)
	}
	return nil
}

func (r *conversationRepository) createGroup(ctx context.Context, name string) (uuid.UUID, error) {
	const query = `INSERT INTO chat_conversations (kind, name, status) VALUES ($1, $2, $3) RETURNING id`
	var id uuid.UUID
	if err := r.db.Conn(ctx).QueryRow(ctx, query, KindGroup, name, StatusApproved).Scan(&id); err != nil {
		return uuid.Nil, fmt.Errorf("create group: %w", err)
	}
	return id, nil
}

func (r *conversationRepository) lock(ctx context.Context, id uuid.UUID) (*lockedRow, error) {
	const query = `
		SELECT id, kind, name, last_seq, change_seq, direct_user_low
		FROM chat_conversations WHERE id = $1 FOR NO KEY UPDATE`
	var c lockedRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, id).Scan(&c.ID, &c.Kind, &c.Name, &c.LastSeq, &c.ChangeSeq, &c.DirectLow)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock conversation: %w", err)
	}
	return &c, nil
}

func (r *conversationRepository) bumpMessage(ctx context.Context, id uuid.UUID) (seq, changeSeq int64, err error) {
	const query = `
		UPDATE chat_conversations
		SET last_seq = last_seq + 1, change_seq = change_seq + 1, last_message_at = NOW()
		WHERE id = $1
		RETURNING last_seq, change_seq`
	if err := r.db.Conn(ctx).QueryRow(ctx, query, id).Scan(&seq, &changeSeq); err != nil {
		return 0, 0, fmt.Errorf("bump conversation: %w", err)
	}
	return seq, changeSeq, nil
}

func (r *conversationRepository) bumpChange(ctx context.Context, id uuid.UUID) (int64, error) {
	const query = `UPDATE chat_conversations SET change_seq = change_seq + 1 WHERE id = $1 RETURNING change_seq`
	var changeSeq int64
	if err := r.db.Conn(ctx).QueryRow(ctx, query, id).Scan(&changeSeq); err != nil {
		return 0, fmt.Errorf("bump conversation change: %w", err)
	}
	return changeSeq, nil
}

func (r *conversationRepository) bumpMessageAsMember(ctx context.Context, id, userID uuid.UUID) (*lockedRow, error) {
	const query = `
		UPDATE chat_conversations c
		SET last_seq = c.last_seq + 1, change_seq = c.change_seq + 1, last_message_at = NOW()
		WHERE c.id = $1 AND EXISTS (SELECT 1 FROM chat_members m WHERE m.conversation_id = c.id AND m.user_id = $2)
		RETURNING c.id, c.kind, c.name, c.last_seq, c.change_seq, c.direct_user_low`
	var c lockedRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, id, userID).Scan(&c.ID, &c.Kind, &c.Name, &c.LastSeq, &c.ChangeSeq, &c.DirectLow)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("bump conversation: %w", err)
	}
	return &c, nil
}

func (r *conversationRepository) rename(ctx context.Context, id uuid.UUID, name string) error {
	const query = `UPDATE chat_conversations SET name = $2 WHERE id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, name); err != nil {
		return fmt.Errorf("rename conversation: %w", err)
	}
	return nil
}

func (r *conversationRepository) delete(ctx context.Context, id uuid.UUID) error {
	const query = `DELETE FROM chat_conversations WHERE id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id); err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return nil
}

func (r *conversationRepository) idsOfUser(ctx context.Context, userID uuid.UUID, kind Kind) ([]uuid.UUID, error) {
	const query = `
		SELECT c.id
		FROM chat_members m
		JOIN chat_conversations c ON c.id = m.conversation_id
		WHERE m.user_id = $1 AND c.kind = $2
		ORDER BY c.id`
	rows, err := r.db.Conn(ctx).Query(ctx, query, userID, kind)
	if err != nil {
		return nil, fmt.Errorf("list user conversations: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan conversation id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list user conversations: %w", err)
	}
	return ids, nil
}

func (r *conversationRepository) lockAsMember(ctx context.Context, id, userID uuid.UUID) (*lockedRow, error) {
	const query = `
		SELECT c.id, c.kind, c.name, c.last_seq, c.change_seq, c.direct_user_low
		FROM chat_conversations c
		WHERE c.id = $1 AND EXISTS (SELECT 1 FROM chat_members m WHERE m.conversation_id = c.id AND m.user_id = $2)
		FOR NO KEY UPDATE OF c`
	var c lockedRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, id, userID).Scan(&c.ID, &c.Kind, &c.Name, &c.LastSeq, &c.ChangeSeq, &c.DirectLow)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("lock conversation: %w", err)
	}
	return &c, nil
}
