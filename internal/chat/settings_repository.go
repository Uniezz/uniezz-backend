package chat

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type settingsRepository struct {
	db database.DB
}

func newSettingsRepository(db database.DB) *settingsRepository {
	return &settingsRepository{db: db}
}

func (r *settingsRepository) get(ctx context.Context, userID uuid.UUID, fallback WhoCanMessage) (WhoCanMessage, error) {
	const query = `SELECT COALESCE((SELECT who_can_message FROM chat_settings WHERE user_id = $1), $2)`
	var who WhoCanMessage
	if err := r.db.Conn(ctx).QueryRow(ctx, query, userID, fallback).Scan(&who); err != nil {
		return "", fmt.Errorf("get chat settings: %w", err)
	}
	return who, nil
}

func (r *settingsRepository) setWhoCanMessage(ctx context.Context, userID uuid.UUID, who WhoCanMessage) error {
	const query = `
		INSERT INTO chat_settings (user_id, who_can_message) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET who_can_message = EXCLUDED.who_can_message`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, who); err != nil {
		return fmt.Errorf("set chat settings: %w", err)
	}
	return nil
}

func (r *settingsRepository) delete(ctx context.Context, userID uuid.UUID) error {
	const query = `DELETE FROM chat_settings WHERE user_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID); err != nil {
		return fmt.Errorf("delete chat settings: %w", err)
	}
	return nil
}
