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
		items = append(items, toQueuedReport(row))
	}
	return items, next, nil
}

type reportChange struct {
	Status ReportStatus
}

func (m Module) updateReport(ctx context.Context, actorID, id uuid.UUID, change reportChange) (*Report, error) {
	action := ActionReviewReport
	switch change.Status {
	case ReportReviewed:
	case ReportDismissed:
		action = ActionDismissReport
	default:
		return nil, ErrInvalidInput
	}
	var row *reportRow
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		row, err = m.queue.resolve(ctx, id, change.Status)
		if err != nil {
			return err
		}
		if err := m.actions.create(ctx, actorID, action, id); err != nil {
			return err
		}
		if change.Status != ReportDismissed || (row.TargetType != targetPost && row.TargetType != targetComment) {
			return nil
		}
		return m.feed.RestoreReported(ctx, row.TargetType, row.TargetID)
	})
	if err != nil {
		return nil, err
	}
	report := toReport(*row)
	return &report, nil
}

func (m Module) report(ctx context.Context, id uuid.UUID) (*QueuedReport, error) {
	row, err := m.queue.get(ctx, id)
	if err != nil {
		return nil, err
	}
	report := toQueuedReport(*row)
	return &report, nil
}

func toQueuedReport(row queueRow) QueuedReport {
	return QueuedReport{Report: toReport(row.reportRow), TargetAuthorID: row.TargetAuthorID, Snapshot: row.Snapshot}
}
