package moderation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
)

type queueRow struct {
	reportRow
	TargetAuthorID *uuid.UUID
	Snapshot       map[string]any
}

type queueCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type queueFilter struct {
	After *queueCursor
	Limit int
}

type queueRepository struct {
	db database.DB
}

func newQueueRepository(db database.DB) *queueRepository {
	return &queueRepository{db: db}
}

func (r *queueRepository) list(ctx context.Context, f queueFilter) ([]queueRow, error) {
	var args database.Args
	arg := args.Add

	// A literal, not a parameter, so a generic plan can still use the partial
	// index on pending reports.
	where := []string{"status = 'pending'"}
	if f.After != nil {
		where = append(where, "(created_at, id) < ("+arg(f.After.CreatedAt)+", "+arg(f.After.ID)+")")
	}

	query := `
		SELECT id, target_type, target_id, target_author_id, reason, details, snapshot, status, created_at
		FROM reports
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY created_at DESC, id DESC
		LIMIT ` + arg(f.Limit)

	rows, err := r.db.Conn(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list pending reports: %w", err)
	}
	defer rows.Close()

	var reports []queueRow
	for rows.Next() {
		var q queueRow
		if err := rows.Scan(&q.ID, &q.TargetType, &q.TargetID, &q.TargetAuthorID, &q.Reason,
			&q.Details, &q.Snapshot, &q.Status, &q.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan pending report: %w", err)
		}
		reports = append(reports, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list pending reports: %w", err)
	}
	return reports, nil
}
