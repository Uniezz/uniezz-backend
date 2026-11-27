package push

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

func (r *settingsRepository) get(ctx context.Context, userID uuid.UUID) (bool, error) {
	const query = `SELECT COALESCE((SELECT enabled FROM push_settings WHERE user_id = $1), TRUE)`
	var enabled bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&enabled); err != nil {
		return false, fmt.Errorf("get push settings: %w", err)
	}
	return enabled, nil
}

func (r *settingsRepository) setEnabled(ctx context.Context, userID uuid.UUID, enabled bool) error {
	const query = `
		INSERT INTO push_settings (user_id, enabled) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET enabled = EXCLUDED.enabled`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, enabled); err != nil {
		return fmt.Errorf("set push settings: %w", err)
	}
	return nil
}
