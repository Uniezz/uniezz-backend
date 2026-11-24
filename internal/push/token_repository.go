package push

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type targetRow struct {
	UserID   uuid.UUID
	Token    string
	Language string
}

type tokenRepository struct {
	db database.DB
}

func newTokenRepository(db database.DB) *tokenRepository {
	return &tokenRepository{db: db}
}

func (r *tokenRepository) deleteOtherOfSession(ctx context.Context, sessionID uuid.UUID, token string) error {
	const query = `DELETE FROM push_tokens WHERE session_id = $1 AND token <> $2`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, sessionID, token); err != nil {
		return fmt.Errorf("delete session push token: %w", err)
	}
	return nil
}

func (r *tokenRepository) upsert(ctx context.Context, sessionID, userID uuid.UUID, token string) error {
	const query = `
		INSERT INTO push_tokens (session_id, user_id, token) VALUES ($1, $2, $3)
		ON CONFLICT (token) DO UPDATE SET session_id = EXCLUDED.session_id, user_id = EXCLUDED.user_id`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, sessionID, userID, token); err != nil {
		return fmt.Errorf("save push token: %w", err)
	}
	return nil
}

func (r *tokenRepository) deleteOfSession(ctx context.Context, sessionID uuid.UUID) error {
	const query = `DELETE FROM push_tokens WHERE session_id = $1`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, sessionID); err != nil {
		return fmt.Errorf("delete push token: %w", err)
	}
	return nil
}

func (r *tokenRepository) deleteMany(ctx context.Context, tokens []string) error {
	const query = `DELETE FROM push_tokens WHERE token = ANY($1)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, tokens); err != nil {
		return fmt.Errorf("delete push tokens: %w", err)
	}
	return nil
}

func (r *tokenRepository) listTargets(ctx context.Context, userIDs []uuid.UUID) ([]targetRow, error) {
	const query = `
		SELECT t.user_id, t.token, l.language
		FROM push_tokens t
		JOIN active_sessions s ON s.id = t.session_id
		JOIN user_languages l ON l.id = t.user_id
		LEFT JOIN push_settings p ON p.user_id = t.user_id
		WHERE t.user_id = ANY($1) AND COALESCE(p.enabled, TRUE)
		ORDER BY t.user_id, t.token`
	rows, err := r.db.Conn(ctx).Query(ctx, query, userIDs)
	if err != nil {
		return nil, fmt.Errorf("list push targets: %w", err)
	}
	defer rows.Close()

	var targets []targetRow
	for rows.Next() {
		var t targetRow
		if err := rows.Scan(&t.UserID, &t.Token, &t.Language); err != nil {
			return nil, fmt.Errorf("scan push target: %w", err)
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list push targets: %w", err)
	}
	return targets, nil
}
