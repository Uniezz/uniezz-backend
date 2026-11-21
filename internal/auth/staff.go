package auth

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) GrantRole(ctx context.Context, userID uuid.UUID, role Role) error {
	found, err := m.roles.grant(ctx, userID, role)
	if err != nil {
		return err
	}
	if !found {
		return ErrUserNotFound
	}
	return nil
}

func (m Module) RevokeRole(ctx context.Context, userID uuid.UUID, role Role) error {
	return m.roles.revoke(ctx, userID, role)
}
