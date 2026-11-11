package profile

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	stuckDeletionAge   = 6 * time.Hour
	resumeDeletionsMax = 20
)

type ResumeDeletionsArgs struct{}

func (ResumeDeletionsArgs) Kind() string { return "profile.resume-deletions.v1" }

// A deletion is stuck once its job has run out of attempts; stuckDeletionAge
// is longer than every retry of one job, so a resumed job never runs next to
// the original. Restarting moves deleted_at forward, so failing deletions go
// to the back of the queue instead of blocking the others.
func (m Module) resumeDeletions(ctx context.Context, _ ResumeDeletionsArgs) error {
	var ids []uuid.UUID
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		ids, err = m.profiles.restartStuckDeletions(ctx, stuckDeletionAge, resumeDeletionsMax)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := m.jobs.Enqueue(ctx, DeleteAccountArgs{UserID: id}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(ids) > 0 {
		m.jobs.Wake()
	}
	return nil
}
