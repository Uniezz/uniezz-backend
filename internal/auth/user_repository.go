package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
