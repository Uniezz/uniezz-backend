package moderation

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

// QueuedReport is a report as a moderator sees it: the public fields plus the
// copy of the reported content. It never names the reporter.
type QueuedReport struct {
	Report
	TargetAuthorID *uuid.UUID     `json:"targetAuthorId" format:"uuid" nullable:"true" doc:"Null once the author's data has been anonymised"`
	Snapshot       map[string]any `json:"snapshot" doc:"Copy of the reported content at the time of the report"`
}

func (m Module) listQueue(ctx context.Context, moderatorID uuid.UUID, cursor string, limit int) ([]QueuedReport, *string, error) {
	ok, err := m.reports.isModerator(ctx, moderatorID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, ErrForbidden
	}

	var after *queueCursor
	if cursor != "" {
		after = &queueCursor{}
		if err := httpapi.DecodeCursor(cursor, after); err != nil {
			return nil, nil, err
		}
	}
	rows, err := m.reports.listPending(ctx, after, limit+1)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, limit, func(last queueRow) any {
		return queueCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	items := make([]QueuedReport, 0, len(rows))
	for _, row := range rows {
		items = append(items, QueuedReport{
			Report: Report{
				ID: row.ID, TargetType: reportTargetType(row.TargetType), TargetID: row.TargetID,
				Reason: row.Reason, Details: row.Details, Status: row.Status, CreatedAt: row.CreatedAt,
			},
			TargetAuthorID: row.TargetAuthorID,
			Snapshot:       row.Snapshot,
		})
	}
	return items, next, nil
}
