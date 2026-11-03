package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Thread struct {
	ID         uuid.UUID
	UserLowID  uuid.UUID
	UserHighID uuid.UUID
	CreatedAt  time.Time
}

type Message struct {
	ID        uuid.UUID
	ThreadID  uuid.UUID
	SenderID  uuid.UUID
	Content   string
	CreatedAt time.Time
}

type repository struct {
	db database.DB
}

func newRepository(db database.DB) *repository {
	return &repository{db: db}
}

func (r *repository) getOrCreateThread(ctx context.Context, userID, otherID uuid.UUID) (*Thread, error) {
	if userID == otherID {
		return nil, ErrInvalidInput
	}

	lowID, highID := userID, otherID
	if lowID.String() > highID.String() {
		lowID, highID = highID, lowID
	}

	_, err := r.db.Conn(ctx).Exec(ctx, `
        INSERT INTO chat_threads (user_low_id, user_high_id)
        VALUES ($1, $2)
        ON CONFLICT (user_low_id, user_high_id) DO NOTHING`,
		lowID, highID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("create chat thread: %w", err)
	}

	var thread Thread
	err = r.db.Conn(ctx).QueryRow(ctx, `
        SELECT id, user_low_id, user_high_id, created_at
        FROM chat_threads
        WHERE user_low_id = $1 AND user_high_id = $2`,
		lowID, highID).Scan(&thread.ID, &thread.UserLowID, &thread.UserHighID, &thread.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get chat thread: %w", err)
	}
	return &thread, nil
}

func (r *repository) listMessages(ctx context.Context, threadID, userID uuid.UUID) ([]Message, error) {
	var isParticipant bool
	err := r.db.Conn(ctx).QueryRow(ctx, `
        SELECT EXISTS (
            SELECT 1 FROM chat_threads
            WHERE id = $1 AND (user_low_id = $2 OR user_high_id = $2)
        )`,
		threadID, userID).Scan(&isParticipant)
	if err != nil {
		return nil, fmt.Errorf("check chat thread access: %w", err)
	}
	if !isParticipant {
		return nil, ErrNotFound
	}

	rows, err := r.db.Conn(ctx).Query(ctx, `
        SELECT id, thread_id, sender_id, content, created_at
        FROM chat_messages
        WHERE thread_id = $1
        ORDER BY created_at, id`,
		threadID)
	if err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	defer rows.Close()

	messages := make([]Message, 0)
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.ID, &message.ThreadID, &message.SenderID, &message.Content, &message.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list chat messages: %w", err)
	}
	return messages, nil
}

func (r *repository) createMessage(ctx context.Context, threadID, senderID uuid.UUID, content string) (*Message, error) {
	var message Message
	err := r.db.Conn(ctx).QueryRow(ctx, `
        INSERT INTO chat_messages (thread_id, sender_id, content)
        SELECT t.id, $2, $3
        FROM chat_threads t
        WHERE t.id = $1 AND (t.user_low_id = $2 OR t.user_high_id = $2)
        RETURNING id, thread_id, sender_id, content, created_at`,
		threadID, senderID, content).Scan(
		&message.ID, &message.ThreadID, &message.SenderID, &message.Content, &message.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("create chat message: %w", err)
	}
	return &message, nil
}
