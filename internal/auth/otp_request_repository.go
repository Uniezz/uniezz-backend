package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type otpRequest struct {
	UniversityID UniversityID
	Email        string
	CodeHash     []byte
}

type otpRequestRepository struct {
	db *pgxpool.Pool
}

func newOTPRequestRepository(db *pgxpool.Pool) *otpRequestRepository {
	return &otpRequestRepository{db: db}
}

func (r *otpRequestRepository) create(ctx context.Context, req otpRequest, ttl time.Duration) error {
	const query = `
		INSERT INTO otp_requests (university_id, email, code_hash, expires_at)
		VALUES ($1, $2, $3, NOW() + make_interval(secs => $4));
	`

	if _, err := r.db.Exec(ctx, query, req.UniversityID, req.Email, req.CodeHash, ttl.Seconds()); err != nil {
		return fmt.Errorf("failed to create otp request: %w", err)
	}
	return nil
}

func (r *otpRequestRepository) verify(ctx context.Context, universityID UniversityID, email string, codeHash []byte) error {
	const query = `
		UPDATE otp_requests
		SET attempts_count = attempts_count + 1,
			consumed_at = CASE WHEN code_hash = $3 THEN NOW() END
		WHERE id = (
			SELECT id FROM otp_requests
			WHERE university_id = $1 AND LOWER(email) = LOWER($2)
			ORDER BY created_at DESC
			LIMIT 1
		) AND consumed_at IS NULL AND expires_at > NOW() AND attempts_count < max_attempts
		RETURNING consumed_at IS NOT NULL;
	`

	var matched bool
	err := r.db.QueryRow(ctx, query, universityID, email, codeHash).Scan(&matched)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCode
		}
		return fmt.Errorf("failed to verify otp request: %w", err)
	}
	if !matched {
		return ErrInvalidCode
	}
	return nil
}
