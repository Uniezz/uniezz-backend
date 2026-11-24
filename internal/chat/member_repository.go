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

type memberRow struct {
	UserID   uuid.UUID
	Role     Role
	JoinedAt time.Time
}

type memberCursor struct {
	NotOwner bool      `json:"o"`
	JoinedAt time.Time `json:"j"`
	UserID   uuid.UUID `json:"u"`
}

type memberFilter struct {
	ConversationID uuid.UUID
	ViewerID       uuid.UUID
	After          *memberCursor
	Limit          int
}

type readRow struct {
	PreviousSeq int64
	LastReadSeq int64
	UnreadCount int
}

type memberRepository struct {
	db database.DB
}

func newMemberRepository(db database.DB) *memberRepository {
	return &memberRepository{db: db}
}

func (r *memberRepository) insert(ctx context.Context, conversationID uuid.UUID, userIDs []uuid.UUID, role Role, readSeq int64) error {
	const query = `
		INSERT INTO chat_members (conversation_id, user_id, role, last_read_seq)
		SELECT $1, id, $3, $4 FROM unnest($2::uuid[]) AS id`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, conversationID, userIDs, role, readSeq); err != nil {
		return fmt.Errorf("insert members: %w", err)
	}
	return nil
}

func (r *memberRepository) list(ctx context.Context, f memberFilter) ([]memberRow, error) {
	args := database.Args{f.ConversationID, f.ViewerID, RoleOwner}
	arg := args.Add

	where := []string{
		"m.conversation_id = $1",
		"NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = $2 AND b.other_id = m.user_id)",
		"NOT EXISTS (SELECT 1 FROM banned_users x WHERE x.id = m.user_id)",
	}
	if f.After != nil {
		where = append(where, "((m.role <> $3), m.joined_at, m.user_id) > ("+arg(f.After.NotOwner)+", "+arg(f.After.JoinedAt)+", "+arg(f.After.UserID)+")")
	}
	query := `
		SELECT m.user_id, m.role, m.joined_at
		FROM chat_members m
		WHERE ` + joinAnd(where) + `
		ORDER BY (m.role <> $3), m.joined_at, m.user_id
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	var members []memberRow
	for rows.Next() {
		var m memberRow
		if err := rows.Scan(&m.UserID, &m.Role, &m.JoinedAt); err != nil {
			return nil, fmt.Errorf("scan member: %w", err)
		}
		members = append(members, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return members, nil
}

func joinAnd(conditions []string) string {
	out := ""
	for i, c := range conditions {
		if i > 0 {
			out += " AND "
		}
		out += c
	}
	return out
}

func (r *memberRepository) count(ctx context.Context, conversationID uuid.UUID) (int, error) {
	const query = `SELECT COUNT(*) FROM chat_members WHERE conversation_id = $1`
	var n int
	if err := r.db.Conn(ctx).QueryRow(ctx, query, conversationID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count members: %w", err)
	}
	return n, nil
}

func (r *memberRepository) userIDs(ctx context.Context, conversationID uuid.UUID) ([]uuid.UUID, error) {
	const query = `SELECT user_id FROM chat_members WHERE conversation_id = $1 ORDER BY user_id`
	return scanUUIDs(ctx, r.db, "list member ids", query, conversationID)
}

func (r *memberRepository) recipients(ctx context.Context, conversationID, actorID uuid.UUID) ([]uuid.UUID, error) {
	const query = `
		SELECT m.user_id
		FROM chat_members m
		WHERE m.conversation_id = $1
		  AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = m.user_id AND b.other_id = $2)
		ORDER BY m.user_id`
	return scanUUIDs(ctx, r.db, "list recipients", query, conversationID, actorID)
}

func (r *memberRepository) existing(ctx context.Context, conversationID uuid.UUID, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	const query = `SELECT user_id FROM chat_members WHERE conversation_id = $1 AND user_id = ANY($2) ORDER BY user_id`
	return scanUUIDs(ctx, r.db, "list existing members", query, conversationID, userIDs)
}

func (r *memberRepository) connected(ctx context.Context, viewerID uuid.UUID, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	const query = `
		SELECT p.other_id
		FROM connected_pairs p
		JOIN public_profiles u ON u.id = p.other_id
		WHERE p.user_id = $1 AND p.other_id = ANY($2)
		  AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = $1 AND b.other_id = p.other_id)
		  AND NOT EXISTS (SELECT 1 FROM banned_users x WHERE x.id = p.other_id)
		ORDER BY p.other_id`
	return scanUUIDs(ctx, r.db, "list connected students", query, viewerID, userIDs)
}

func (r *memberRepository) delete(ctx context.Context, conversationID, userID uuid.UUID) error {
	const query = `DELETE FROM chat_members WHERE conversation_id = $1 AND user_id = $2`
	result, err := r.db.Conn(ctx).Exec(ctx, query, conversationID, userID)
	if err != nil {
		return fmt.Errorf("delete member: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *memberRepository) longestStanding(ctx context.Context, conversationID uuid.UUID) (uuid.UUID, error) {
	const query = `
		SELECT user_id FROM chat_members
		WHERE conversation_id = $1
		ORDER BY joined_at, user_id
		LIMIT 1`
	var id uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, conversationID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("find longest standing member: %w", err)
	}
	return id, nil
}

func (r *memberRepository) setRole(ctx context.Context, conversationID, userID uuid.UUID, role Role) error {
	const query = `UPDATE chat_members SET role = $3 WHERE conversation_id = $1 AND user_id = $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, conversationID, userID, role); err != nil {
		return fmt.Errorf("set member role: %w", err)
	}
	return nil
}

func (r *memberRepository) countUnread(ctx context.Context, conversationID, senderID uuid.UUID) error {
	const query = `
		UPDATE chat_members m
		SET unread_count = m.unread_count + 1
		WHERE m.conversation_id = $1 AND m.user_id <> $2
		  AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = m.user_id AND b.other_id = $2)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, conversationID, senderID); err != nil {
		return fmt.Errorf("count unread: %w", err)
	}
	return nil
}

func (r *memberRepository) markReadThrough(ctx context.Context, conversationID, userID uuid.UUID, seq int64) error {
	const query = `
		UPDATE chat_members
		SET last_read_seq = GREATEST(last_read_seq, $3), unread_count = 0
		WHERE conversation_id = $1 AND user_id = $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, conversationID, userID, seq); err != nil {
		return fmt.Errorf("advance read position: %w", err)
	}
	return nil
}

func (r *memberRepository) markRead(ctx context.Context, conversationID, userID uuid.UUID, seq, lastSeq int64) (*readRow, error) {
	query := `
		UPDATE chat_members m
		SET last_read_seq = GREATEST(m.last_read_seq, $3),
			unread_count = CASE WHEN GREATEST(m.last_read_seq, $3) >= $4 THEN 0 ELSE (
				SELECT COUNT(*) FROM chat_messages msg
				WHERE msg.conversation_id = m.conversation_id
				  AND msg.seq > GREATEST(m.last_read_seq, $3)
				  AND msg.sender_id IS DISTINCT FROM m.user_id
				  AND ` + messageVisible("m.user_id", "msg.sender_id") + `
			) END
		FROM chat_members old
		WHERE m.conversation_id = $1 AND m.user_id = $2
		  AND old.conversation_id = m.conversation_id AND old.user_id = m.user_id
		RETURNING old.last_read_seq, m.last_read_seq, m.unread_count`

	var read readRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, conversationID, userID, seq, lastSeq).Scan(&read.PreviousSeq, &read.LastReadSeq, &read.UnreadCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("mark read: %w", err)
	}
	return &read, nil
}

func (r *memberRepository) unreadTotals(ctx context.Context, viewerID uuid.UUID) (total, conversations int, err error) {
	args := database.Args{viewerID}
	query := `
		SELECT COALESCE(SUM(m.unread_count), 0), COUNT(*) FILTER (WHERE m.unread_count > 0)
		FROM chat_members m
		JOIN chat_conversations c ON c.id = m.conversation_id
		WHERE m.user_id = $1 AND ` + directVisible(args.Add, "$1", true)
	if err := r.db.Conn(ctx).QueryRow(ctx, query, args...).Scan(&total, &conversations); err != nil {
		return 0, 0, fmt.Errorf("count unread conversations: %w", err)
	}
	return total, conversations, nil
}
