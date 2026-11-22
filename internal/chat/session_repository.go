package chat

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type sessionRepository struct {
	db database.DB
}

func newSessionRepository(db database.DB) *sessionRepository {
	return &sessionRepository{db: db}
}

func (r *sessionRepository) active(ctx context.Context, sessionID uuid.UUID) (bool, error) {
	const query = `SELECT EXISTS (SELECT 1 FROM active_sessions WHERE id = $1)`
	var active bool
	if err := r.db.Conn(ctx).QueryRow(ctx, query, sessionID).Scan(&active); err != nil {
		return false, fmt.Errorf("check session: %w", err)
	}
	return active, nil
}
