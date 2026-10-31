package media

import (
	"context"
	"errors"
	"time"
)

const (
	abandonedUploadAge = time.Hour
	cleanupBatch       = 200
)

type CleanupArgs struct{}

func (CleanupArgs) Kind() string { return "media.cleanup.v1" }

func (m Module) cleanup(ctx context.Context, _ CleanupArgs) error {
	if err := m.repo.markAbandonedDeleting(ctx, abandonedUploadAge); err != nil {
		return err
	}

	ids, err := m.repo.listDeleting(ctx, cleanupBatch)
	if err != nil {
		return err
	}

	var errs []error
	for _, id := range ids {
		if err := m.Delete(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
