package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type banRepository struct {
	db database.DB
}

func newBanRepository(db database.DB) *banRepository {
	return &banRepository{db: db}
}

func (r *banRepository) ban(ctx context.Context, userID uuid.UUID) (bool, error) {
	// The lock orders a ban against a role granted at the same time, so staff
	// are never banned.
	const query = `
		WITH target AS (
			SELECT u.id, EXISTS (SELECT 1 FROM user_roles r WHERE r.user_id = u.id) AS staff
			FROM users u
			WHERE u.id = $1 AND u.deleted_at IS NULL
			FOR UPDATE
		),
		banned AS (
			UPDATE users SET banned_at = NOW()
			WHERE id IN (SELECT id FROM target WHERE NOT staff) AND banned_at IS NULL
		)
		SELECT staff FROM target`
	var staff bool
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&staff)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrUserNotFound
	}
	if err != nil {
		return false, fmt.Errorf("ban user: %w", err)
	}
	return staff, nil
}

func (r *banRepository) unban(ctx context.Context, userID uuid.UUID) error {
	const query = `UPDATE users SET banned_at = NULL WHERE id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID); err != nil {
		return fmt.Errorf("unban user: %w", err)
	}
	return nil
}

func (r *banRepository) bannedAt(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	const query = `SELECT banned_at FROM users WHERE id = $1 AND deleted_at IS NULL`
	var at *time.Time
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get ban: %w", err)
	}
	return at, nil
}
