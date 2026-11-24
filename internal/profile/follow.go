package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

type followListQuery struct {
	ViewerID  uuid.UUID
	ProfileID uuid.UUID
	Following bool
	Cursor    string
	Limit     int
}

func (m Module) follow(ctx context.Context, followerID, followedID uuid.UUID) error {
	if followerID == followedID {
		return ErrInvalidInput
	}

	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.follows.lockPair(ctx, followerID, followedID); err != nil {
			return err
		}
		if _, err := m.profiles.get(ctx, followedID); err != nil {
			return err
		}
		hidden, err := m.profiles.hidden(ctx, followerID, followedID)
		if err != nil {
			return err
		}
		if hidden {
			return ErrNotFound
		}
		return m.follows.insert(ctx, followerID, followedID)
	})
}

func (m Module) unfollow(ctx context.Context, followerID, followedID uuid.UUID) error {
	if followerID == followedID {
		return ErrInvalidInput
	}

	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.follows.lockPair(ctx, followerID, followedID); err != nil {
			return err
		}
		if _, err := m.profiles.get(ctx, followedID); err != nil {
			return err
		}
		return m.follows.delete(ctx, followerID, followedID)
	})
}

func (m Module) listFollows(ctx context.Context, q followListQuery) ([]ProfileSummary, *string, error) {
	if _, err := m.profiles.get(ctx, q.ProfileID); err != nil {
		return nil, nil, err
	}
	hidden, err := m.profiles.hidden(ctx, q.ViewerID, q.ProfileID)
	if err != nil {
		return nil, nil, err
	}
	if hidden {
		return nil, nil, ErrNotFound
	}

	filter := followFilter{ProfileID: q.ProfileID, ViewerID: q.ViewerID, Following: q.Following, Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after followCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}
	rows, err := m.follows.list(ctx, filter.ProfileID, filter.ViewerID, filter.Following, filter.After, filter.Limit)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last followRow) any {
		return followCursor{CreatedAt: last.CreatedAt, UserID: last.UserID}
	})
	if err != nil {
		return nil, nil, err
	}

	userIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	profiles, err := m.profiles.getMany(ctx, userIDs)
	if err != nil {
		return nil, nil, err
	}
	byID := make(map[uuid.UUID]profileRow, len(profiles))
	for _, p := range profiles {
		byID[p.ID] = p
	}

	items := make([]ProfileSummary, 0, len(rows))
	for _, row := range rows {
		if p, ok := byID[row.UserID]; ok {
			items = append(items, m.summary(ctx, p))
		}
	}
	return items, next, nil
}

type followFilter struct {
	ProfileID uuid.UUID
	ViewerID  uuid.UUID
	Following bool
	After     *followCursor
	Limit     int
}

// LockFollowPair serializes follows with a block between the same users.
func (m Module) LockFollowPair(ctx context.Context, firstID, secondID uuid.UUID) error {
	return m.follows.lockPair(ctx, firstID, secondID)
}

// RemoveFollowsBetween removes either direction of a follow for a blocked pair.
func (m Module) RemoveFollowsBetween(ctx context.Context, firstID, secondID uuid.UUID) error {
	return m.follows.removeBetween(ctx, firstID, secondID)
}
