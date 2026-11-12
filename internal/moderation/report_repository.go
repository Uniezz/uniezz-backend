package moderation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type reportRow struct {
	ID         uuid.UUID
	TargetType string
	TargetID   uuid.UUID
	Reason     string
	Details    *string
	Status     string
	CreatedAt  time.Time
}

type newReport struct {
	ReporterID     uuid.UUID
	TargetType     string
	TargetID       uuid.UUID
	TargetAuthorID uuid.UUID
	Reason         string
	Details        *string
	Snapshot       map[string]any
}

type reportRepository struct {
	db database.DB
}

func newReportRepository(db database.DB) *reportRepository {
	return &reportRepository{db: db}
}

func (r *reportRepository) exists(ctx context.Context, reporterID uuid.UUID, targetType string, targetID uuid.UUID) (bool, error) {
	var found bool
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM reports WHERE reporter_id = $1 AND target_type = $2 AND target_id = $3)`,
		reporterID, targetType, targetID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check report: %w", err)
	}
	return found, nil
}

func (r *reportRepository) insert(ctx context.Context, n newReport) (*reportRow, error) {
	row := reportRow{TargetType: n.TargetType, TargetID: n.TargetID, Reason: n.Reason, Details: n.Details}
	err := r.db.Conn(ctx).QueryRow(ctx, `
		INSERT INTO reports (reporter_id, target_type, target_id, target_author_id, reason, details, snapshot)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (reporter_id, target_type, target_id) DO NOTHING
		RETURNING id, status, created_at`,
		n.ReporterID, n.TargetType, n.TargetID, n.TargetAuthorID, n.Reason, n.Details, n.Snapshot).
		Scan(&row.ID, &row.Status, &row.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlreadyReport
		}
		return nil, fmt.Errorf("insert report: %w", err)
	}
	return &row, nil
}

func (r *reportRepository) markTargetsForgotten(ctx context.Context) error {
	const query = `
		UPDATE reports r SET target_forgotten_at = NOW()
		WHERE r.target_author_id IS NOT NULL
		  AND r.target_author_id <> $1
		  AND r.target_forgotten_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM public_profiles p WHERE p.id = r.target_author_id)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, uuid.Nil); err != nil {
		return fmt.Errorf("mark forgotten report targets: %w", err)
	}
	return nil
}

func (r *reportRepository) anonymiseForgotten(ctx context.Context, age time.Duration) error {
	const query = `
		UPDATE reports SET snapshot = '{}', target_author_id = NULL
		WHERE target_forgotten_at < NOW() - $1::interval
		  AND (snapshot <> '{}' OR target_author_id IS NOT NULL)`
	if _, err := r.db.Conn(ctx).Exec(ctx, query, age); err != nil {
		return fmt.Errorf("anonymise reports: %w", err)
	}
	return nil
}

func (r *reportRepository) isModerator(ctx context.Context, userID uuid.UUID) (bool, error) {
	var found bool
	err := r.db.Conn(ctx).QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM moderators m JOIN users u ON u.id = m.user_id
			WHERE m.user_id = $1 AND u.deleted_at IS NULL)`, userID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("check moderator: %w", err)
	}
	return found, nil
}

func (r *reportRepository) listPending(ctx context.Context, after *queueCursor, limit int) ([]queueRow, error) {
	args := database.Args{}
	where := "status = 'pending'"
	if after != nil {
		where += " AND (created_at, id) < (" + args.Add(after.CreatedAt) + ", " + args.Add(after.ID) + ")"
	}
	query := `
		SELECT id, target_type, target_id, target_author_id, reason, details, snapshot, status, created_at
		FROM reports WHERE ` + where + `
		ORDER BY created_at DESC, id DESC LIMIT ` + args.Add(limit)

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
