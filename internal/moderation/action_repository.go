package moderation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type actionRow struct {
	ID        uuid.UUID
	ActorID   *uuid.UUID
	Type      ActionType
	TargetID  uuid.UUID
	CreatedAt time.Time
}

type actionCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type actionFilter struct {
	After *actionCursor
	Limit int
}

type actionRepository struct {
	db database.DB
}

func newActionRepository(db database.DB) *actionRepository {
	return &actionRepository{db: db}
}

func (r *actionRepository) create(ctx context.Context, actorID uuid.UUID, actionType ActionType, targetID uuid.UUID) error {
	const query = `INSERT INTO moderation_actions (actor_id, type, target_id) VALUES ($1, $2, $3)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, actorID, actionType, targetID); err != nil {
		return fmt.Errorf("create moderation action: %w", err)
	}
	return nil
}

func (r *actionRepository) list(ctx context.Context, f actionFilter) ([]actionRow, error) {
	var args database.Args
	arg := args.Add

	var where []string
	if f.After != nil {
		where = append(where, "(created_at, id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}
	query := `SELECT id, actor_id, type, target_id, created_at FROM moderation_actions`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list moderation actions: %w", err)
	}
	defer rows.Close()

	var actions []actionRow
	for rows.Next() {
		var row actionRow
		if err := rows.Scan(&row.ID, &row.ActorID, &row.Type, &row.TargetID, &row.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan moderation action: %w", err)
		}
		actions = append(actions, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list moderation actions: %w", err)
	}
	return actions, nil
}

func (r *actionRepository) prune(ctx context.Context, age time.Duration) error {
	const query = `DELETE FROM moderation_actions WHERE created_at < NOW() - $1::interval`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, age); err != nil {
		return fmt.Errorf("prune moderation actions: %w", err)
	}
	return nil
}
