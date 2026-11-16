package auth

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

const inviteRedeemWindow = 7 * 24 * time.Hour

type inviteChange struct {
	Code string
}

func (m Module) inviteCode(ctx context.Context, userID uuid.UUID) (string, error) {
	return m.logins.users.inviteCode(ctx, userID)
}

func (m Module) redeemInvite(ctx context.Context, userID uuid.UUID, change inviteChange) (int, error) {
	code := strings.ToUpper(strings.TrimSpace(change.Code))
	if !validInviteCode(code) {
		return 0, ErrInvalidReferralCode
	}

	earned := 0
	err := m.logins.tx.WithTx(ctx, func(ctx context.Context) error {
		inviterID, err := m.logins.users.inviter(ctx, code, userID)
		if err != nil {
			return err
		}
		set, err := m.logins.users.setInviter(ctx, userID, inviterID, inviteRedeemWindow)
		if err != nil {
			return err
		}
		if !set {
			current, isOpen, err := m.logins.users.inviterState(ctx, userID, inviteRedeemWindow)
			switch {
			case err != nil:
				return err
			case current != nil && *current == inviterID:
				return nil
			case current != nil:
				return ErrReferralAlreadySet
			case !isOpen:
				return ErrReferralWindowClosed
			}
			return ErrReferralAlreadySet
		}
		earned, err = m.logins.referrals.RewardReferral(ctx, userID, inviterID)
		return err
	})
	return earned, err
}
