package moderation

import (
	"context"
	"errors"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

type moderatorQuery struct {
	Cursor string
	Limit  int
}

func (m Module) grantModerator(ctx context.Context, userID uuid.UUID) error {
	err := m.staff.GrantRole(ctx, userID, auth.RoleModerator)
	if errors.Is(err, auth.ErrUserNotFound) {
		return ErrNotFound
	}
	return err
}

func (m Module) revokeModerator(ctx context.Context, userID uuid.UUID) error {
	return m.staff.RevokeRole(ctx, userID, auth.RoleModerator)
}

func (m Module) listModerators(ctx context.Context, q moderatorQuery) ([]Moderator, *string, error) {
	filter := moderatorFilter{Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after moderatorCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.moderators.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}

	rows, next, err := httpapi.Page(rows, q.Limit, func(last moderatorRow) any {
		return moderatorCursor{GrantedAt: last.GrantedAt, UserID: last.UserID}
	})
	if err != nil {
		return nil, nil, err
	}

	items := make([]Moderator, 0, len(rows))
	for _, row := range rows {
		items = append(items, Moderator{UserID: row.UserID, FirstName: row.FirstName, LastName: row.LastName, GrantedAt: row.GrantedAt})
	}
	return items, next, nil
}
