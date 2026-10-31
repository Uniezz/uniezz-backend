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
