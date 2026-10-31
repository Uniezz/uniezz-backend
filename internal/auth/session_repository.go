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

type Session struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash []byte
	ExpiresAt time.Time
	CreatedAt time.Time
}

type SessionRepository struct {
	db database.DB
}

func NewSessionRepository(db database.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

func (r *SessionRepository) CreateSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, expiresAt time.Time) (*Session, error) {
	const query = `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, expires_at, created_at;
		`

	var s Session
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID, tokenHash, expiresAt).Scan(
		&s.ID,
		&s.UserID,
		&s.TokenHash,
		&s.ExpiresAt,
		&s.CreatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("session token collision: %w", err)
		}
		return nil, fmt.Errorf("failed to insert session: %w", err)
	}

	return &s, nil
}

func (r *SessionRepository) GetActiveSessionByHash(
	ctx context.Context,
	tokenHash []byte,
) (*Session, error) {
	const query = `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM sessions
		WHERE token_hash = $1 AND expires_at > NOW();
	`

	var s Session
	err := r.db.Conn(ctx).QueryRow(ctx, query, tokenHash).Scan(
		&s.ID,
		&s.UserID,
		&s.TokenHash,
		&s.ExpiresAt,
		&s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to query active session: %w", err)
	}

	return &s, nil
}

func (r *SessionRepository) ExtendSession(ctx context.Context, id uuid.UUID, expiresAt time.Time) error {
	const query = `UPDATE sessions SET expires_at = $2 WHERE id = $1 AND expires_at < $2`

	if _, err := r.db.Conn(ctx).Exec(ctx, query, id, expiresAt); err != nil {
		return fmt.Errorf("failed to extend session: %w", err)
	}
	return nil
}
