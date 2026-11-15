package auth

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) inviteCode(ctx context.Context, userID uuid.UUID) (string, error) {
	return m.logins.users.inviteCode(ctx, userID)
}
