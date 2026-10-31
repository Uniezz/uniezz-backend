package profile

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type profileRow struct {
	ID            uuid.UUID
	UniversityID  string
	Email         *string
	FirstName     *string
	LastName      *string
	AvatarMediaID *uuid.UUID
	CreatedAt     time.Time
}

type profileRepository struct {
	db database.DB
}

func newProfileRepository(db *pgxpool.Pool) *profileRepository {
	return &profileRepository{db: database.New(db)}
}

func (r *profileRepository) get(ctx context.Context, userID uuid.UUID) (*profileRow, error) {
	const query = `
		SELECT id, university_id, email, first_name, last_name, avatar_media_id, created_at
		FROM users
		WHERE id = $1`

	var p profileRow
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID).Scan(&p.ID, &p.UniversityID, &p.Email, &p.FirstName, &p.LastName, &p.AvatarMediaID, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get profile: %w", err)
	}
	return &p, nil
}

func (r *profileRepository) updateNames(ctx context.Context, userID uuid.UUID, firstName, lastName *string) error {
	const query = `
		UPDATE users
		SET first_name = COALESCE($2, first_name), last_name = COALESCE($3, last_name)
		WHERE id = $1`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, userID, firstName, lastName); err != nil {
		return fmt.Errorf("update names: %w", err)
	}
	return nil
}

func (r *profileRepository) replaceAvatar(ctx context.Context, userID uuid.UUID, mediaID *uuid.UUID) (*uuid.UUID, error) {
	var previous *uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, `SELECT avatar_media_id FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&previous)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("lock avatar: %w", err)
	}

	if _, err := r.db.Conn(ctx).Exec(ctx, `UPDATE users SET avatar_media_id = $2 WHERE id = $1`, userID, mediaID); err != nil {
		return nil, fmt.Errorf("set avatar: %w", err)
	}
	return previous, nil
}

func (r *profileRepository) setAvatarIfEmpty(ctx context.Context, userID, mediaID uuid.UUID) (bool, error) {
	result, err := r.db.Conn(ctx).Exec(ctx, `UPDATE users SET avatar_media_id = $2 WHERE id = $1 AND avatar_media_id IS NULL`, userID, mediaID)
	if err != nil {
		return false, fmt.Errorf("set imported avatar: %w", err)
	}
	return result.RowsAffected() == 1, nil
}
