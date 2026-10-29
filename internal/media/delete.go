package media

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (m Module) MarkDeleting(ctx context.Context, tx pgx.Tx, mediaID uuid.UUID) error {
	return m.repo.markDeleting(ctx, tx, mediaID)
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
