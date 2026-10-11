package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/usos"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type usosStateRepository struct {
	db *pgxpool.Pool
}

func newUsosStateRepository(db *pgxpool.Pool) *usosStateRepository {
	return &usosStateRepository{db: db}
}

func (r *usosStateRepository) save(ctx context.Context, s usosState) error {
	const query = `
		INSERT INTO oauth_states (request_token, request_secret, university_id, platform)
		VALUES ($1, $2, $3, $4);
	`

	_, err := r.db.Exec(ctx, query, s.RequestToken.Token, s.RequestToken.Secret, s.UniversityID, s.Platform)
	if err != nil {
		return fmt.Errorf("failed to save usos state: %w", err)
	}
	return nil
}

func (r *usosStateRepository) take(ctx context.Context, requestToken string) (usosState, error) {
	const query = `
		DELETE FROM oauth_states
		WHERE request_token = $1
		RETURNING request_secret, university_id, platform, expires_at > NOW();
	`

	s := usosState{RequestToken: usos.RequestToken{Token: requestToken}}
	var active bool
	err := r.db.QueryRow(ctx, query, requestToken).Scan(
		&s.RequestToken.Secret,
		&s.UniversityID,
		&s.Platform,
		&active,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return usosState{}, ErrStateNotFound
		}
		return usosState{}, fmt.Errorf("failed to take usos state: %w", err)
	}
	if !active {
		return usosState{}, fmt.Errorf("%w: expired", ErrStateNotFound)
	}

	return s, nil
}
