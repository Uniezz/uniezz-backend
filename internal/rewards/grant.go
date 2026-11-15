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
	ReasonDailyQuests     Reason = "daily_quests"
	ReasonFAQApproved     Reason = "faq_approved"
)

var amounts = map[Reason]int{
	ReasonWelcome:         100,
	ReasonLikeGiven:       1,
	ReasonLikeReceived:    1,
	ReasonComment:         1,
	ReasonProfileComplete: 50,
	ReasonBirthDate:       10,
	ReasonReferral:        100,
	ReasonDailyQuests:     10,
	ReasonFAQApproved:     5,
}

// Grant pays userID for one action, at most once per (reason, refID, actorID), and returns the coins
// it paid: 0 when the action was already paid. Call it inside the transaction of the write that earns
// the coins, so a rolled-back write pays nothing.
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
		return m.coins.addToBalance(ctx, userID, amount)
	})
	return paid, err
}

func (m Module) Balance(ctx context.Context, userID uuid.UUID) (int, error) {
	return m.coins.balance(ctx, userID)
}
