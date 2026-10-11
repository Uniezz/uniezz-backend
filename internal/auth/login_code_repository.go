package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type loginCodeRepository struct {
	db *pgxpool.Pool
}

func newLoginCodeRepository(db *pgxpool.Pool) *loginCodeRepository {
	return &loginCodeRepository{db: db}
}

func (r *loginCodeRepository) create(ctx context.Context, userID uuid.UUID, codeHash []byte) error {
	const query = `INSERT INTO login_codes (code_hash, user_id) VALUES ($1, $2);`

	if _, err := r.db.Exec(ctx, query, codeHash, userID); err != nil {
		return fmt.Errorf("failed to create login code: %w", err)
	}
	return nil
}

func (r *loginCodeRepository) consume(ctx context.Context, codeHash []byte) (uuid.UUID, error) {
	const query = `
		DELETE FROM login_codes
		WHERE code_hash = $1
		RETURNING user_id, expires_at > NOW();
	`

	var userID uuid.UUID
	var active bool
	err := r.db.QueryRow(ctx, query, codeHash).Scan(&userID, &active)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrInvalidCode
		}
		return uuid.Nil, fmt.Errorf("failed to consume login code: %w", err)
	}
	if !active {
		return uuid.Nil, fmt.Errorf("%w: expired", ErrInvalidCode)
	}

	return userID, nil
}
