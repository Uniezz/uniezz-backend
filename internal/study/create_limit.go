package study

import (
	"context"
	"errors"

	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/google/uuid"
)

func (m Module) allowCreate(ctx context.Context, userID uuid.UUID) error {
	err := m.limiter.Allow(ctx, userID.String())
	if errors.Is(err, ratelimit.ErrLimited) {
		return errors.Join(ErrRateLimited, err)
	}
	return err
}
