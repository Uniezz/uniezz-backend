package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	inviteCodeLength    = 7
	inviteAlphabet      = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

type User struct {
	ID           uuid.UUID
	UniversityID UniversityID
	UsosUserID   *string
	Email        *string
	FirstName    *string
	LastName     *string
	LastLoginAt  *time.Time
	InviteCode   string
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
					invite_code,
					created_at,
					updated_at`

const upsertByUsosIDQuery = `
						INSERT INTO users (university_id, usos_user_id, email, first_name, last_name, invite_code, last_login_at)
						VALUES ($1, $2, $3, $4, $5, $6, NOW())
						ON CONFLICT (university_id, usos_user_id) WHERE usos_user_id IS NOT NULL
						DO UPDATE SET
							email         = COALESCE(users.email, EXCLUDED.email),
							first_name    = COALESCE(users.first_name, EXCLUDED.first_name),
							last_name     = COALESCE(users.last_name, EXCLUDED.last_name),
							last_login_at = NOW()
						RETURNING ` + userColumns + `, (xmax = 0)`

const upsertByEmailQuery = `
							INSERT INTO users (university_id, email, first_name, last_name, invite_code, last_login_at)
							VALUES ($1, $2, $3, $4, $5, NOW())
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
	for {
		inviteCode, err := generateInviteCode()
		if err != nil {
			return nil, false, fmt.Errorf("generate invite code: %w", err)
		}

		var row pgx.Row
		if id.UsosUserID != "" {
			row = r.db.Conn(ctx).QueryRow(ctx, upsertByUsosIDQuery,
				id.UniversityID, id.UsosUserID, nullIfEmpty(id.Email), nullIfEmpty(id.FirstName), nullIfEmpty(id.LastName), inviteCode)
		} else {
			row = r.db.Conn(ctx).QueryRow(ctx, upsertByEmailQuery,
				id.UniversityID, id.Email, nullIfEmpty(id.FirstName), nullIfEmpty(id.LastName), inviteCode)
		}

		var u User
		var created bool
		err = row.Scan(
			&u.ID,
			&u.UniversityID,
			&u.UsosUserID,
			&u.Email,
			&u.FirstName,
			&u.LastName,
			&u.LastLoginAt,
			&u.InviteCode,
			&u.CreatedAt,
			&u.UpdatedAt,
			&created,
		)
		if err == nil {
			return &u, created, nil
		}
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == uniqueViolation && postgresError.ConstraintName == "uq_users_invite_code" {
			continue
		}
		return nil, false, fmt.Errorf("upsert user: %w", err)
	}
}

func generateInviteCode() (string, error) {
	code := make([]byte, inviteCodeLength)
	for i := range code {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(inviteAlphabet))))
		if err != nil {
			return "", err
		}
		code[i] = inviteAlphabet[index.Int64()]
	}
	return string(code), nil
}

func validInviteCode(code string) bool {
	if len(code) != inviteCodeLength {
		return false
	}
	for _, char := range code {
		if !strings.ContainsRune(inviteAlphabet, char) {
			return false
		}
	}
	return true
}

func (r *UserRepository) inviter(ctx context.Context, code string, userID uuid.UUID) (uuid.UUID, error) {
	const query = `
		SELECT id FROM users
		WHERE invite_code = $1 AND deleted_at IS NULL
			AND id <> $2 AND invited_by IS DISTINCT FROM $2`
	var inviterID uuid.UUID
	err := r.db.Conn(ctx).QueryRow(ctx, query, code, userID).Scan(&inviterID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrInvalidReferralCode
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("get invite owner: %w", err)
	}
	return inviterID, nil
}

func (r *UserRepository) setInviter(ctx context.Context, userID, inviterID uuid.UUID, window time.Duration) (bool, error) {
	const query = `
		UPDATE users SET invited_by = $2
		WHERE id = $1 AND deleted_at IS NULL AND invited_by IS NULL
			AND created_at > NOW() - $3::interval`
	tag, err := r.db.Conn(ctx).Exec(ctx, query, userID, inviterID, window)
	if err != nil {
		return false, fmt.Errorf("set inviter: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *UserRepository) inviterState(ctx context.Context, userID uuid.UUID, window time.Duration) (*uuid.UUID, bool, error) {
	const query = `
		SELECT invited_by, created_at > NOW() - $2::interval
		FROM users WHERE id = $1 AND deleted_at IS NULL`
	var inviterID *uuid.UUID
	var isOpen bool
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID, window).Scan(&inviterID, &isOpen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrSessionNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("get inviter: %w", err)
	}
	return inviterID, isOpen, nil
}

func (r *UserRepository) inviteCode(ctx context.Context, userID uuid.UUID) (string, error) {
	var code string
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT invite_code FROM users WHERE id = $1 AND deleted_at IS NULL`, userID).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get invite code: %w", err)
	}
	return code, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
