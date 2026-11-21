package moderation

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

type queueQuery struct {
	Cursor string
	Limit  int
}

func (m Module) listQueue(ctx context.Context, q queueQuery) ([]QueuedReport, *string, error) {
	filter := queueFilter{Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after queueCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.queue.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}

	rows, next, err := httpapi.Page(rows, q.Limit, func(last queueRow) any {
		return queueCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	items := make([]QueuedReport, 0, len(rows))
	for _, row := range rows {
		items = append(items, QueuedReport{
			Report:         toReport(row.reportRow),
			TargetAuthorID: row.TargetAuthorID,
			Snapshot:       row.Snapshot,
		})
	}
	return items, next, nil
}

type reportChange struct {
	Status ReportStatus
}

func (m Module) updateReport(ctx context.Context, id uuid.UUID, change reportChange) (*Report, error) {
	if change.Status != ReportReviewed && change.Status != ReportDismissed {
		return nil, ErrInvalidInput
	}
	row, err := m.queue.resolve(ctx, id, change.Status)
	if err != nil {
		return nil, err
	}
	report := toReport(*row)
	return &report, nil
}
