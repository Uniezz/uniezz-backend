package account

import (
	"context"
	"errors"
	"time"
)

const (
	stuckDeletionAge   = time.Hour
	resumeDeletionsMax = 20
)

type ResumeDeletionsArgs struct{}

func (ResumeDeletionsArgs) Kind() string { return "account.resume-deletions.v1" }

// resumeDeletions runs the cleanup again for users whose deletion started
// over an hour ago and has not finished, for example because its job ran out
// of attempts. A job still retrying at the same time is harmless: every step
// can run twice.
func (m Module) resumeDeletions(ctx context.Context, _ ResumeDeletionsArgs) error {
	ids, err := m.accounts.StuckDeletions(ctx, stuckDeletionAge, resumeDeletionsMax)
	if err != nil {
		return err
	}

	var errs []error
	for _, id := range ids {
		if err := m.deleteAccount(ctx, DeleteAccountArgs{UserID: id}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
