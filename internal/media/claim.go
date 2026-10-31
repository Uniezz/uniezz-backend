package media

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) Claim(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind Kind) error {
	if err := m.CheckUpload(ctx, mediaID, ownerUserID, kind); err != nil {
		return err
	}
	return m.MarkReady(ctx, mediaID, ownerUserID, kind)
}

func (m Module) CheckUpload(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind Kind) error {
	item, err := m.repo.getOwnedPending(ctx, mediaID, ownerUserID, kind)
	if err != nil {
		return err
	}

	size, contentType, err := m.storage.headObject(ctx, item.ObjectKey)
	if err != nil {
		return err
	}
	if size != item.SizeBytes || contentType != item.ContentType {
		return ErrUploadMismatch
	}
	return nil
}

func (m Module) MarkReady(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind Kind) error {
	return m.repo.markReady(ctx, mediaID, ownerUserID, kind)
}
