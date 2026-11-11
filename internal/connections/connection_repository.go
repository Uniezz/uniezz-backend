package connections

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type connectionRow struct {
	ID        uuid.UUID
	OtherID   uuid.UUID
	Mode      Mode
	CreatedAt time.Time
}

type connectionCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type connectionRepository struct {
	db database.DB
}

func newConnectionRepository(db database.DB) *connectionRepository {
	return &connectionRepository{db: db}
}

func (r *connectionRepository) create(ctx context.Context, userID, otherID uuid.UUID, mode Mode) (*connectionRow, error) {
	const query = `
		INSERT INTO connections (user_low, user_high, mode)
		VALUES (LEAST($1, $2)::uuid, GREATEST($1, $2)::uuid, $3)
		RETURNING id, mode, created_at`
	c := connectionRow{OtherID: otherID}
	err := r.db.Conn(ctx).QueryRow(ctx, query, userID, otherID, mode).Scan(&c.ID, &c.Mode, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create connection: %w", err)
	}
	return &c, nil
}

func (r *connectionRepository) list(ctx context.Context, userID uuid.UUID, after *connectionCursor, limit int) ([]connectionRow, error) {
	args := database.Args{userID}
	arg := args.Add

	where := ""
	if after != nil {
		where = " AND (c.created_at, c.id) < (" + arg(after.CreatedAt) + ", " + arg(after.ID) + ")"
	}
	query := `
		SELECT c.id, other.id, c.mode, c.created_at
		FROM connections c
		CROSS JOIN LATERAL (SELECT CASE WHEN c.user_low = $1 THEN c.user_high ELSE c.user_low END AS id) other
		WHERE (c.user_low = $1 OR c.user_high = $1)
		  AND NOT EXISTS (SELECT 1 FROM blocked_pairs b WHERE b.user_id = $1 AND b.other_id = other.id)` + where + `
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT ` + arg(limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	defer rows.Close()

	var connections []connectionRow
	for rows.Next() {
		var c connectionRow
		if err := rows.Scan(&c.ID, &c.OtherID, &c.Mode, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan connection: %w", err)
		}
		connections = append(connections, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	return connections, nil
}

func (r *connectionRepository) delete(ctx context.Context, userID, id uuid.UUID) error {
	const query = `DELETE FROM connections WHERE id = $1 AND (user_low = $2 OR user_high = $2)`
	result, err := r.db.Conn(ctx).Exec(ctx, query, id, userID)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
