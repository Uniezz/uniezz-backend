package media

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

func (m Module) ForgetUser(ctx context.Context, userID uuid.UUID) error {
	ids, err := m.repo.markOwnerDeleting(ctx, userID)
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
