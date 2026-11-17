package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/google/uuid"
)

var _ auth.ReferralRewarder = Module{}

func (m Module) RewardReferral(ctx context.Context, userID, inviterID uuid.UUID) (int, error) {
	if _, err := m.rewards.Grant(ctx, inviterID, rewards.ReasonReferral, &userID, &userID); err != nil {
		return 0, err
	}
	return m.rewards.Grant(ctx, userID, rewards.ReasonReferralJoined, &userID, &inviterID)
}
