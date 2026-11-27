package chat

import (
	"context"

	"github.com/google/uuid"
)

type PruneLinkPreviewsArgs struct{}

func (PruneLinkPreviewsArgs) Kind() string { return "chat.prune-link-previews.v1" }

func (m Module) pruneLinkPreviews(ctx context.Context, _ PruneLinkPreviewsArgs) error {
	var mediaIDs []uuid.UUID
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		mediaIDs, err = m.previews.prune(ctx, previewFreshFor, previewRetryAfter)
		if err != nil || len(mediaIDs) == 0 {
			return err
		}
		return m.media.MarkDeletingMany(ctx, mediaIDs)
	})
	if err != nil {
		return err
	}
	m.releaseMedia(ctx, mediaIDs)
	return nil
}
