package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type requestFilter struct {
	SenderID    uuid.UUID
	RecipientID uuid.UUID
	AdultAge    int
}

type inboxRow struct {
	SenderIsNew bool
	Pending     int
}

type requestRepository struct {
	db database.DB
}

func newRequestRepository(db database.DB) *requestRepository {
	return &requestRepository{db: db}
}

func (r *requestRepository) canStart(ctx context.Context, f requestFilter) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM public_profiles o
			JOIN public_profiles v ON v.id = $1
			LEFT JOIN chat_settings s ON s.user_id = o.id
			LEFT JOIN connection_preferences vp ON vp.user_id = v.id
			LEFT JOIN connection_preferences op ON op.user_id = o.id
			WHERE o.id = $2
			  AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = $1 AND b.other_id = $2)
			  AND NOT EXISTS (SELECT 1 FROM banned_users x WHERE x.id = $2)
			  AND (COALESCE(s.who_can_message, $3) = $3 OR (s.who_can_message = $4 AND o.university_id = v.university_id))
			  AND NOT COALESCE(
			      vp.birth_date <= CURRENT_DATE - make_interval(years => $5)
			      AND op.birth_date > CURRENT_DATE - make_interval(years => $5), false))`
	var ok bool
	err := r.db.Conn(ctx).QueryRow(ctx, query, f.SenderID, f.RecipientID, WhoEveryone, WhoUniversity, f.AdultAge).Scan(&ok)
	if err != nil {
		return false, fmt.Errorf("check message request: %w", err)
	}
	return ok, nil
}

func (r *requestRepository) newAccountInbox(ctx context.Context, conversationID, senderID uuid.UUID, newAccountAge time.Duration) (*inboxRow, error) {
	const query = `
		WITH target AS (
			SELECT CASE WHEN direct_user_low = $2 THEN direct_user_high ELSE direct_user_low END AS recipient_id
			FROM chat_conversations WHERE id = $1)
		SELECT
			EXISTS (SELECT 1 FROM public_profiles p WHERE p.id = $2 AND p.created_at > NOW() - $3::interval),
			(SELECT COUNT(*)
			 FROM target t
			 JOIN chat_conversations c ON c.direct_user_low = t.recipient_id OR c.direct_user_high = t.recipient_id
			 JOIN public_profiles s ON s.id = c.started_by
			 WHERE c.id <> $1 AND c.status = $4 AND c.started_by <> t.recipient_id AND c.last_seq > 0
			   AND s.created_at > NOW() - $3::interval)`
	var inbox inboxRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, conversationID, senderID, newAccountAge, StatusPending).Scan(&inbox.SenderIsNew, &inbox.Pending)
	if err != nil {
		return nil, fmt.Errorf("count message requests: %w", err)
	}
	return &inbox, nil
}

func (r *requestRepository) accept(ctx context.Context, viewerID, id uuid.UUID) error {
	const query = `
		UPDATE chat_conversations c SET status = $3
		WHERE c.id = $1 AND c.kind = $4 AND c.started_by <> $2 AND c.status = $5 AND c.last_seq > 0
		  AND EXISTS (SELECT 1 FROM chat_members m WHERE m.conversation_id = c.id AND m.user_id = $2)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, viewerID, StatusApproved, KindDirect, StatusPending); err != nil {
		return fmt.Errorf("accept message request: %w", err)
	}
	return nil
}

func (r *requestRepository) decline(ctx context.Context, viewerID, id uuid.UUID) error {
	const query = `
		UPDATE chat_conversations c SET status = $3
		WHERE c.id = $1 AND c.kind = $4 AND c.started_by <> $2 AND c.status IN ($3, $5) AND c.last_seq > 0
		  AND EXISTS (SELECT 1 FROM chat_members m WHERE m.conversation_id = c.id AND m.user_id = $2)`
	result, err := r.db.Conn(ctx).Exec(ctx, query, id, viewerID, StatusDeclined, KindDirect, StatusPending)
	if err != nil {
		return fmt.Errorf("decline message request: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
