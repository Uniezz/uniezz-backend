package connections

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const preferencesColumns = `birth_date, open_to, intent`

type preferencesRow struct {
	BirthDate *time.Time
	OpenTo    []Mode
	Intent    *string
}

type preferencesFields struct {
	BirthDate *time.Time
	OpenTo    []Mode
	SetIntent bool
	Intent    *string
}

func scanPreferences(row pgx.Row) (*preferencesRow, error) {
	var p preferencesRow
	if err := row.Scan(&p.BirthDate, &p.OpenTo, &p.Intent); err != nil {
		return nil, err
	}
	return &p, nil
}

type preferencesRepository struct {
	db database.DB
}

func newPreferencesRepository(db database.DB) *preferencesRepository {
	return &preferencesRepository{db: db}
}

func (r *preferencesRepository) get(ctx context.Context, userID uuid.UUID) (*preferencesRow, error) {
	const query = `SELECT ` + preferencesColumns + ` FROM connection_profiles WHERE user_id = $1`
	p, err := scanPreferences(r.db.Conn(ctx).QueryRow(ctx, query, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get connection preferences: %w", err)
	}
	return p, nil
}

func (r *preferencesRepository) lockOrCreate(ctx context.Context, userID uuid.UUID) (*preferencesRow, error) {
	const insert = `INSERT INTO connection_profiles (user_id) VALUES ($1) ON CONFLICT DO NOTHING`
	if _, err := r.db.Conn(ctx).Exec(ctx, insert, userID); err != nil {
		return nil, fmt.Errorf("create connection preferences: %w", err)
	}

	const query = `SELECT ` + preferencesColumns + ` FROM connection_profiles WHERE user_id = $1 FOR NO KEY UPDATE`
	p, err := scanPreferences(r.db.Conn(ctx).QueryRow(ctx, query, userID))
	if err != nil {
		return nil, fmt.Errorf("lock connection preferences: %w", err)
	}
	return p, nil
}

func (r *preferencesRepository) update(ctx context.Context, userID uuid.UUID, f preferencesFields) (*preferencesRow, error) {
	const query = `
		UPDATE connection_profiles
		SET birth_date = COALESCE($2, birth_date),
			open_to = COALESCE($3, open_to),
			intent = CASE WHEN $4 THEN $5 ELSE intent END
		WHERE user_id = $1
		RETURNING ` + preferencesColumns
	p, err := scanPreferences(r.db.Conn(ctx).QueryRow(ctx, query, userID, f.BirthDate, f.OpenTo, f.SetIntent, f.Intent))
	if err != nil {
		return nil, fmt.Errorf("update connection preferences: %w", err)
	}
	return p, nil
}
