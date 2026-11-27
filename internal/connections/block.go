package connections

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if blockerID == blockedID {
		return ErrInvalidInput
	}
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.blocks.insert(ctx, blockerID, blockedID); err != nil {
			return err
		}
		return m.profiles.RemoveFollowsBetween(ctx, blockerID, blockedID)
	})
}

func (m Module) unblock(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	return m.blocks.delete(ctx, blockerID, blockedID)
}
