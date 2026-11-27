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
		if err := m.follows.lockUsers(ctx, followerID, followedID); err != nil {
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
		inserted, err := m.follows.insert(ctx, followerID, followedID)
		if err != nil || !inserted {
			return err
		}
		return m.follows.increment(ctx, followerID, followedID)
	})
}

func (m Module) unfollow(ctx context.Context, followerID, followedID uuid.UUID) error {
	if followerID == followedID {
		return ErrInvalidInput
	}

	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		deleted, err := m.follows.delete(ctx, followerID, followedID)
		if err != nil || !deleted {
			return err
		}
		return m.subtractFollows(ctx, []followPair{{FollowerID: followerID, FollowedID: followedID}})
	})
}

func (m Module) subtractFollows(ctx context.Context, pairs []followPair) error {
	if len(pairs) == 0 {
		return nil
	}

	byUser := make(map[uuid.UUID]followCountChange)
	for _, p := range pairs {
		followed := byUser[p.FollowedID]
		followed.UserID = p.FollowedID
		followed.Followers++
		byUser[p.FollowedID] = followed

		follower := byUser[p.FollowerID]
		follower.UserID = p.FollowerID
		follower.Following++
		byUser[p.FollowerID] = follower
	}

	userIDs := make([]uuid.UUID, 0, len(byUser))
	changes := make([]followCountChange, 0, len(byUser))
	for id, c := range byUser {
		userIDs = append(userIDs, id)
		changes = append(changes, c)
	}
	if err := m.follows.lockCounts(ctx, userIDs); err != nil {
		return err
	}
	return m.follows.subtract(ctx, changes)
}

func (m Module) forgetFollows(ctx context.Context, userID uuid.UUID) error {
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.follows.lockUsers(ctx, userID); err != nil {
			return err
		}
		pairs, err := m.follows.deleteAllOf(ctx, userID)
		if err != nil {
			return err
		}
		return m.subtractFollows(ctx, pairs)
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
	rows, err := m.follows.list(ctx, filter)
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
