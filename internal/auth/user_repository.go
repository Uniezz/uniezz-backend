package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
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
						RETURNING ` + userColumns

const upsertByEmailQuery = `
							INSERT INTO users (university_id, email, first_name, last_name, last_login_at)
							VALUES ($1, $2, $3, $4, NOW())
							ON CONFLICT (university_id, LOWER(email)) WHERE email IS NOT NULL
							DO UPDATE SET
								first_name    = COALESCE(users.first_name, EXCLUDED.first_name),
								last_name     = COALESCE(users.last_name, EXCLUDED.last_name),
								last_login_at = NOW()
							RETURNING ` + userColumns

func (r *UserRepository) Upsert(ctx context.Context, id Identity) (*User, error) {
	var row pgx.Row
	if id.UsosUserID != "" {
		row = r.db.QueryRow(ctx, upsertByUsosIDQuery,
			id.UniversityID, id.UsosUserID, nullIfEmpty(id.Email), nullIfEmpty(id.FirstName), nullIfEmpty(id.LastName))
	} else {
		row = r.db.QueryRow(ctx, upsertByEmailQuery,
			id.UniversityID, id.Email, nullIfEmpty(id.FirstName), nullIfEmpty(id.LastName))
	}

	var u User
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
	)
	if err != nil {
		return nil, fmt.Errorf("failed to upsert user: %w", err)
	}

	return &u, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
