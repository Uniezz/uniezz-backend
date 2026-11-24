package connections

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

func (m Module) list(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]Connection, *string, error) {
	var after *connectionCursor
	if cursor != "" {
		after = &connectionCursor{}
		if err := httpapi.DecodeCursor(cursor, after); err != nil {
			return nil, nil, err
		}
	}

	rows, err := m.connections.list(ctx, userID, after, limit+1)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, limit, func(last connectionRow) any {
		return connectionCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	items, err := m.withProfiles(ctx, rows)
	if err != nil {
		return nil, nil, err
	}
	return items, next, nil
}

func (m Module) withProfiles(ctx context.Context, rows []connectionRow) ([]Connection, error) {
	userIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.OtherID)
	}
	summaries, err := m.profiles.Summaries(ctx, userIDs)
	if err != nil {
		return nil, err
	}

	items := make([]Connection, 0, len(rows))
	for _, row := range rows {
		summary, ok := summaries[row.OtherID]
		if !ok {
			continue
		}
		items = append(items, Connection{ID: row.ID, Mode: row.Mode, ConnectedAt: row.CreatedAt, Profile: summary})
	}
	return items, nil
}

func (m Module) deleteConnection(ctx context.Context, userID, id uuid.UUID) error {
	return m.connections.delete(ctx, userID, id)
}
