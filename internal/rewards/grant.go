package rewards

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type Reason string

const (
	ReasonWelcome         Reason = "welcome"
	ReasonLikeGiven       Reason = "like_given"
	ReasonLikeReceived    Reason = "like_received"
	ReasonComment         Reason = "comment"
	ReasonProfileComplete Reason = "profile_complete"
	ReasonBirthDate       Reason = "birth_date"
	ReasonReferral        Reason = "referral"
	ReasonReferralJoined  Reason = "referral_joined"
	ReasonDailyQuests     Reason = "daily_quests"
	ReasonFAQApproved     Reason = "faq_approved"
	ReasonChallenge       Reason = "challenge"
)

var amounts = map[Reason]int{
	ReasonWelcome:         100,
	ReasonLikeGiven:       1,
	ReasonLikeReceived:    1,
	ReasonComment:         1,
	ReasonProfileComplete: 50,
	ReasonBirthDate:       10,
	ReasonReferral:        100,
	ReasonReferralJoined:  50,
	ReasonDailyQuests:     10,
	ReasonFAQApproved:     5,
}

// Grant pays userID for one action, at most once per (reason, refID, actorID), counts a paid action
// towards the student's challenges, and returns the coins it paid, challenge rewards included: 0 when
// the action was already paid. Call it inside the transaction of the write that earns the coins, so a
// rolled-back write pays nothing.
func (m Module) Grant(ctx context.Context, userID uuid.UUID, reason Reason, refID, actorID *uuid.UUID) (int, error) {
	amount, ok := amounts[reason]
	if !ok {
		return 0, fmt.Errorf("grant coins: unknown reason %q", reason)
	}

	paid := 0
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		added, err := m.coins.addGrant(ctx, userID, reason, refID, actorID, amount)
		if err != nil || !added {
			return err
		}
		paid = amount
		if err := m.coins.addToBalance(ctx, userID, amount); err != nil {
			return err
		}
		action, ok := grantActions[reason]
		if !ok {
			return nil
		}
		bonus, err := m.Track(ctx, userID, action)
		paid += bonus
		return err
	})
	return paid, err
}

func (m Module) Balance(ctx context.Context, userID uuid.UUID) (int, error) {
	return m.coins.balance(ctx, userID)
}
