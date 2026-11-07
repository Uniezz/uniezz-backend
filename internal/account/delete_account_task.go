package account

import (
	"context"

	"github.com/google/uuid"
)

type DeleteAccountArgs struct {
	UserID uuid.UUID `json:"userId"`
}

func (DeleteAccountArgs) Kind() string { return "account.delete.v1" }

// deleteAccount removes what the user left, in an order that keeps everyone
// else's data consistent: the feed first, because its posts hold media; then
// the media; then the reports about the user; and last the user row, which
// refuses to go while anything still points at it. Profile data has no
// service logic and goes with the row: programmes cascade and the avatar
// pointer is cleared when its media is deleted. Every step can run again, so a
// failed attempt is simply retried from the start.
func (m Module) deleteAccount(ctx context.Context, args DeleteAccountArgs) error {
	steps := []func(context.Context, uuid.UUID) error{
		m.feed.ForgetUser,
		m.media.ForgetUser,
		m.moderation.ForgetUser,
		m.accounts.FinishDeletion,
	}
	for _, step := range steps {
		if err := step(ctx, args.UserID); err != nil {
			return err
		}
	}
	return nil
}
