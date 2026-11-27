package profile

import (
	"context"

	"github.com/google/uuid"
)

// RemoveFollowsBetween removes both directions of a follow inside the caller's
// block transaction. It locks both students first, so a follow that races the
// block either commits before and is removed here, or waits and sees the block.
func (m Module) RemoveFollowsBetween(ctx context.Context, firstID, secondID uuid.UUID) error {
	if err := m.follows.lockUsers(ctx, firstID, secondID); err != nil {
		return err
	}
	pairs, err := m.follows.deleteBetween(ctx, firstID, secondID)
	if err != nil {
		return err
	}
	return m.subtractFollows(ctx, pairs)
}
