package profile

import (
	"context"

	"github.com/google/uuid"
)

type DeleteAccountArgs struct {
	UserID uuid.UUID `json:"userId"`
}

func (DeleteAccountArgs) Kind() string { return "profile.delete-account.v1" }

// The feed and chat go before media because posts and messages hold media, and
// the user row goes last because the feed, study, chat and media foreign keys
// refuse the delete while anything is left.
func (m Module) deleteAccountTask(ctx context.Context, args DeleteAccountArgs) error {
	deleting, err := m.profiles.isDeleting(ctx, args.UserID)
	if err != nil || !deleting {
		return err
	}

	if err := m.feed.ForgetUser(ctx, args.UserID); err != nil {
		return err
	}
	if err := m.study.ForgetUser(ctx, args.UserID); err != nil {
		return err
	}
	if err := m.chat.ForgetUser(ctx, args.UserID); err != nil {
		return err
	}
	if err := m.media.ForgetUser(ctx, args.UserID); err != nil {
		return err
	}
	if err := m.forgetFollows(ctx, args.UserID); err != nil {
		return err
	}
	return m.profiles.deleteDeleting(ctx, args.UserID)
}
