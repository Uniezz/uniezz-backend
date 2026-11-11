package connections

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) block(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	if blockerID == blockedID {
		return ErrInvalidInput
	}
	return m.blocks.insert(ctx, blockerID, blockedID)
}
