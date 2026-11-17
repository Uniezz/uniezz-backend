package rewards

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

func (m Module) grants(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]CoinGrant, *string, error) {
	var after *grantCursor
	if cursor != "" {
		after = &grantCursor{}
		if err := httpapi.DecodeCursor(cursor, after); err != nil {
			return nil, nil, err
		}
	}

	rows, err := m.coins.listGrants(ctx, userID, after, limit+1)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, limit, func(last grantRow) any {
		return grantCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	items := make([]CoinGrant, 0, len(rows))
	for _, row := range rows {
		items = append(items, CoinGrant{Reason: row.Reason, Amount: row.Amount, CreatedAt: row.CreatedAt})
	}
	return items, next, nil
}
