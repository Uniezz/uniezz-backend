package connections

import (
	"context"
	"errors"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const foreignKeyViolation = "23503"

type blockRepository struct {
	db database.DB
}

func newBlockRepository(db database.DB) *blockRepository {
	return &blockRepository{db: db}
}

func (r *blockRepository) insert(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	const query = `INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := r.db.Conn(ctx).Exec(ctx, query, blockerID, blockedID)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("block student: %w", err)
	}
	return nil
}

func (r *blockRepository) delete(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	const query = `DELETE FROM blocks WHERE blocker_id = $1 AND blocked_id = $2`
	_, err := r.db.Conn(ctx).Exec(ctx, query, blockerID, blockedID)
	if err != nil {
		return fmt.Errorf("unblock student: %w", err)
	}
	return nil
}
