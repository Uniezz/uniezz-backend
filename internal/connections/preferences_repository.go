package connections

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type preferencesRow struct {
	BirthDate *time.Time
	OpenTo    []string
	Intent    *string
}

type preferencesRepository struct {
	db database.DB
}

func newPreferencesRepository(db database.DB) *preferencesRepository {
	return &preferencesRepository{db: db}
}

func (r *preferencesRepository) lock(ctx context.Context, userID uuid.UUID) (*preferencesRow, error) {
	_, err := r.db.Conn(ctx).Exec(ctx, `INSERT INTO connection_profiles (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID)
	if err != nil {
		return nil, fmt.Errorf("create connection profile: %w", err)
	}

	var p preferencesRow
	err = r.db.Conn(ctx).QueryRow(ctx, `
		SELECT birth_date, open_to, intent FROM connection_profiles
		WHERE user_id = $1 FOR NO KEY UPDATE`, userID).Scan(&p.BirthDate, &p.OpenTo, &p.Intent)
	if err != nil {
		return nil, fmt.Errorf("lock connection profile: %w", err)
	}
	return &p, nil
}

func (r *preferencesRepository) update(ctx context.Context, userID uuid.UUID, p preferencesRow) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
		UPDATE connection_profiles SET birth_date = $2, open_to = $3, intent = $4
		WHERE user_id = $1`, userID, p.BirthDate, p.OpenTo, p.Intent)
	if err != nil {
		return fmt.Errorf("save connection preferences: %w", err)
	}
	return nil
}
