package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
)

func (m Module) Ban(ctx context.Context, userID uuid.UUID) error {
	staff, err := m.bans.ban(ctx, userID)
	if err != nil {
		return err
	}
	if staff {
		return ErrStaff
	}
	return nil
}

func (m Module) Unban(ctx context.Context, userID uuid.UUID) error {
	return m.bans.unban(ctx, userID)
}

func (m Module) BannedAt(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	return m.bans.bannedAt(ctx, userID)
}
