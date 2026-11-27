package chat

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type eventRepository struct {
	db database.DB
}

func newEventRepository(db database.DB) *eventRepository {
	return &eventRepository{db: db}
}

func (r *eventRepository) setAnswer(ctx context.Context, messageID, userID uuid.UUID, answer EventAnswer) (bool, error) {
	const query = `
		INSERT INTO chat_event_answers (message_id, user_id, answer)
		VALUES ($1, $2, $3)
		ON CONFLICT (message_id, user_id) DO UPDATE SET answer = EXCLUDED.answer
		WHERE chat_event_answers.answer <> EXCLUDED.answer`
	result, err := r.db.Conn(ctx).Exec(ctx, query, messageID, userID, answer)
	if err != nil {
		return false, fmt.Errorf("set event answer: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

func (r *eventRepository) deleteAnswer(ctx context.Context, messageID, userID uuid.UUID) (bool, error) {
	const query = `DELETE FROM chat_event_answers WHERE message_id = $1 AND user_id = $2`
	result, err := r.db.Conn(ctx).Exec(ctx, query, messageID, userID)
	if err != nil {
		return false, fmt.Errorf("delete event answer: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

func (r *eventRepository) deleteAnswers(ctx context.Context, messageID uuid.UUID) error {
	const query = `DELETE FROM chat_event_answers WHERE message_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, messageID); err != nil {
		return fmt.Errorf("delete event answers: %w", err)
	}
	return nil
}

func (r *eventRepository) cancel(ctx context.Context, messageID uuid.UUID) (bool, error) {
	const query = `
		UPDATE chat_messages
		SET payload = jsonb_set(payload, '{canceledAt}', to_jsonb(NOW()))
		WHERE id = $1 AND kind = $2 AND deleted_at IS NULL AND payload->'canceledAt' = 'null'::jsonb`
	result, err := r.db.Conn(ctx).Exec(ctx, query, messageID, MessageEvent)
	if err != nil {
		return false, fmt.Errorf("cancel event: %w", err)
	}
	return result.RowsAffected() > 0, nil
}

func (r *eventRepository) counts(ctx context.Context, messageIDs []uuid.UUID) (map[uuid.UUID]map[EventAnswer]int, error) {
	const query = `
		SELECT message_id, answer, COUNT(*)
		FROM chat_event_answers
		WHERE message_id = ANY($1)
		GROUP BY message_id, answer`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs)
	if err != nil {
		return nil, fmt.Errorf("count event answers: %w", err)
	}
	defer rows.Close()

	counts := map[uuid.UUID]map[EventAnswer]int{}
	for rows.Next() {
		var id uuid.UUID
		var answer EventAnswer
		var n int
		if err := rows.Scan(&id, &answer, &n); err != nil {
			return nil, fmt.Errorf("scan event count: %w", err)
		}
		if counts[id] == nil {
			counts[id] = map[EventAnswer]int{}
		}
		counts[id][answer] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count event answers: %w", err)
	}
	return counts, nil
}

func (r *eventRepository) answersOf(ctx context.Context, messageIDs []uuid.UUID, userID uuid.UUID) (map[uuid.UUID]EventAnswer, error) {
	const query = `SELECT message_id, answer FROM chat_event_answers WHERE message_id = ANY($1) AND user_id = $2`
	rows, err := r.db.Conn(ctx).Query(ctx, query, messageIDs, userID)
	if err != nil {
		return nil, fmt.Errorf("list event answers: %w", err)
	}
	defer rows.Close()

	answers := map[uuid.UUID]EventAnswer{}
	for rows.Next() {
		var id uuid.UUID
		var answer EventAnswer
		if err := rows.Scan(&id, &answer); err != nil {
			return nil, fmt.Errorf("scan event answer: %w", err)
		}
		answers[id] = answer
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list event answers: %w", err)
	}
	return answers, nil
}

func (r *eventRepository) recentAttendees(ctx context.Context, messageIDs []uuid.UUID, viewerID uuid.UUID, limit int) (map[uuid.UUID][]uuid.UUID, error) {
	const query = `
		SELECT message_id, user_id
		FROM (
			SELECT a.message_id, a.user_id,
				ROW_NUMBER() OVER (PARTITION BY a.message_id ORDER BY a.updated_at DESC, a.user_id) AS n
			FROM chat_event_answers a
			WHERE a.message_id = ANY($1) AND a.answer = ANY($4)
			  AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = $2 AND b.other_id = a.user_id)
			  AND NOT EXISTS (SELECT 1 FROM banned_users x WHERE x.id = a.user_id)
		) ranked
		WHERE n <= $3
		ORDER BY message_id, n`
	return scanVoters(ctx, r.db, "list event attendees", query, messageIDs, viewerID, limit, []EventAnswer{AnswerGoing, AnswerMaybe})
}
