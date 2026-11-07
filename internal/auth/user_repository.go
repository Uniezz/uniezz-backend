package auth

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

type User struct {
	ID           uuid.UUID
	UniversityID UniversityID
	UsosUserID   *string
	Email        *string
	FirstName    *string
	LastName     *string
	LastLoginAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UserRepository struct {
	db database.DB
}

func NewUserRepository(db database.DB) *UserRepository {
	return &UserRepository{db: db}
}

const userColumns = `id,
					university_id,
					usos_user_id, email,
					first_name, last_name,
					last_login_at,
					created_at,
					updated_at`

const upsertByUsosIDQuery = `
						INSERT INTO users (university_id, usos_user_id, email, first_name, last_name, last_login_at)
						VALUES ($1, $2, $3, $4, $5, NOW())
						ON CONFLICT (university_id, usos_user_id) WHERE usos_user_id IS NOT NULL
						DO UPDATE SET
							email         = COALESCE(users.email, EXCLUDED.email),
							first_name    = COALESCE(users.first_name, EXCLUDED.first_name),
							last_name     = COALESCE(users.last_name, EXCLUDED.last_name),
							last_login_at = NOW()
						RETURNING ` + userColumns + `, (xmax = 0)`

const upsertByEmailQuery = `
							INSERT INTO users (university_id, email, first_name, last_name, last_login_at)
							VALUES ($1, $2, $3, $4, NOW())
							ON CONFLICT (university_id, LOWER(email)) WHERE email IS NOT NULL
							DO UPDATE SET
								first_name    = COALESCE(users.first_name, EXCLUDED.first_name),
								last_name     = COALESCE(users.last_name, EXCLUDED.last_name),
								last_login_at = NOW()
							RETURNING ` + userColumns + `, (xmax = 0)`

func (r *UserRepository) Upsert(ctx context.Context, id Identity) (*User, error) {
	u, _, err := r.upsert(ctx, id)
	return u, err
}

func (r *UserRepository) upsert(ctx context.Context, id Identity) (*User, bool, error) {
	var row pgx.Row
	if id.UsosUserID != "" {
		row = r.db.Conn(ctx).QueryRow(ctx, upsertByUsosIDQuery,
			id.UniversityID, id.UsosUserID, nullIfEmpty(id.Email), nullIfEmpty(id.FirstName), nullIfEmpty(id.LastName))
	} else {
		row = r.db.Conn(ctx).QueryRow(ctx, upsertByEmailQuery,
			id.UniversityID, id.Email, nullIfEmpty(id.FirstName), nullIfEmpty(id.LastName))
	}

	var u User
	var created bool
	err := row.Scan(
		&u.ID,
		&u.UniversityID,
		&u.UsosUserID,
		&u.Email,
		&u.FirstName,
		&u.LastName,
		&u.LastLoginAt,
		&u.CreatedAt,
		&u.UpdatedAt,
		&created,
	)
	if err != nil {
		return nil, false, fmt.Errorf("failed to upsert user: %w", err)
	}

	return &u, created, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (r *UserRepository) markDeleting(ctx context.Context, userID uuid.UUID) (bool, error) {
	const query = `
		UPDATE users
		SET deleted_at = NOW(), email = NULL, usos_user_id = NULL
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := r.db.Conn(ctx).Exec(ctx, query, userID)
	if err != nil {
		return false, fmt.Errorf("failed to mark user as deleting: %w", err)
	}
	return result.RowsAffected() == 1, nil
}

func (r *UserRepository) deleteDeleting(ctx context.Context, userID uuid.UUID) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `DELETE FROM users WHERE id = $1 AND deleted_at IS NOT NULL`, userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return fmt.Errorf("%w: %w", errStillReferenced, err)
		}
		return fmt.Errorf("failed to delete user: %w", err)
	}
	return nil
}

func (r *UserRepository) listDeletingSince(ctx context.Context, olderThan time.Duration, limit int) ([]uuid.UUID, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
		SELECT id FROM users
		WHERE deleted_at < NOW() - $1::interval
		ORDER BY deleted_at
		LIMIT $2`, olderThan, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list users being deleted: %w", err)
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, fmt.Errorf("failed to list users being deleted: %w", err)
	}
	return ids, nil
}
