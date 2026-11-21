package rewards

import (
	"context"

	"github.com/google/uuid"
)

var grantActions = map[Reason]Action{
	ReasonLikeGiven:       ActionLikeGiven,
	ReasonLikeReceived:    ActionLikeReceived,
	ReasonComment:         ActionCommentPosted,
	ReasonProfileComplete: ActionProfileCompleted,
	ReasonReferral:        ActionFriendInvited,
	ReasonFAQApproved:     ActionFAQApproved,
}

var questActions = map[Quest]Action{
	QuestReply: ActionReplyPosted,
	QuestPost:  ActionPostCreated,
}

// Track counts one action towards the student's active challenges, pays every challenge it completes,
// and returns the coins it paid. Call it inside the transaction of the action, once per action.
func (m Module) Track(ctx context.Context, userID uuid.UUID, action Action) (int, error) {
	paid := 0
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		completed, err := m.challenges.step(ctx, userID, action, questZone)
		if err != nil || len(completed) == 0 {
			return err
		}
		refs := make([]uuid.UUID, 0, len(completed))
		amounts := make([]int, 0, len(completed))
		for _, c := range completed {
			refs = append(refs, c.ProgressID)
			amounts = append(amounts, c.Reward)
		}
		paid, err = m.coins.addGrants(ctx, userID, ReasonChallenge, refs, amounts)
		if err != nil || paid == 0 {
			return err
		}
		return m.coins.addToBalance(ctx, userID, paid)
	})
	return paid, err
}
