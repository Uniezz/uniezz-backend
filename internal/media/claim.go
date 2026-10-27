package media

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

type pendingMediaRepository interface {
	getOwnedPending(ctx context.Context, id, ownerUserID uuid.UUID, kind Kind) (*Media, error)
	markReady(ctx context.Context, id, ownerUserID uuid.UUID) error
}

type objectInspector interface {
	HeadObject(ctx context.Context, key string) (int64, string, error)
}

func claimUploadedMedia(
	ctx context.Context,
	repo pendingMediaRepository,
	storage objectInspector,
	mediaID uuid.UUID,
	ownerUserID uuid.UUID,
	kind Kind,
) error {
	item, err := repo.getOwnedPending(ctx, mediaID, ownerUserID, kind)
	if err != nil {
		return err
	}

	size, contentType, err := storage.HeadObject(ctx, item.ObjectKey)
	if err != nil {
		return err
	}
	if size != item.SizeBytes {
		return fmt.Errorf("uploaded media size does not match")
	}
	if contentType != item.ContentType {
		return fmt.Errorf("uploaded media content type does not match")
	}

	if err := repo.markReady(ctx, mediaID, ownerUserID); err != nil {
		return err
	}
	return nil
}
