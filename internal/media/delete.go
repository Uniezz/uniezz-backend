package media

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

func (m Module) MarkDeleting(ctx context.Context, mediaID uuid.UUID) error {
	return m.repo.markDeleting(ctx, mediaID)
}

func (m Module) Delete(ctx context.Context, mediaID uuid.UUID) error {
	item, err := m.repo.getDeleting(ctx, mediaID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := m.storage.deleteObject(ctx, item.ObjectKey); err != nil {
		return err
	}
	return m.repo.deleteDeleting(ctx, mediaID)
}
