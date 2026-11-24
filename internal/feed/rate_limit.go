package feed

import (
	"context"
	"errors"

	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/google/uuid"
)

func allow(ctx context.Context, userID uuid.UUID, limiters ...limiter) error {
	for _, l := range limiters {
		err := l.Allow(ctx, userID.String())
		if errors.Is(err, ratelimit.ErrLimited) {
			return errors.Join(ErrRateLimited, err)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
