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

func (m Module) grantModerator(ctx context.Context, actorID, userID uuid.UUID) error {
	err := m.staff.GrantRole(ctx, userID, auth.RoleModerator)
	if errors.Is(err, auth.ErrUserNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return m.actions.create(ctx, actorID, ActionGrantModerator, userID)
}

func (m Module) revokeModerator(ctx context.Context, actorID, userID uuid.UUID) error {
	if err := m.staff.RevokeRole(ctx, userID, auth.RoleModerator); err != nil {
		return err
	}
	return m.actions.create(ctx, actorID, ActionRevokeModerator, userID)
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
		items = append(items, Moderator(row))
	}
	return items, next, nil
}
