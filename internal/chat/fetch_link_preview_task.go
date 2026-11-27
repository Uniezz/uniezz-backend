package chat

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

type FetchLinkPreviewArgs struct {
	MessageID uuid.UUID `json:"messageId"`
}

func (FetchLinkPreviewArgs) Kind() string { return "chat.fetch-link-preview.v1" }

func (m Module) fetchLinkPreview(ctx context.Context, args FetchLinkPreviewArgs) error {
	link, err := m.previews.getLink(ctx, args.MessageID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	fresh, err := m.previews.fresh(ctx, link.URLHash, previewFreshFor, previewRetryAfter)
	if err != nil {
		return err
	}
	var notes []notification
	if fresh {
		ready, err := m.previews.previews(ctx, []uuid.UUID{link.MessageID})
		if err != nil || len(ready) == 0 {
			return err
		}
		notes, err = m.announcePreview(ctx, link.MessageID)
		if err != nil {
			return err
		}
	} else {
		var fields previewFields
		fields, err = m.fetchPreview(ctx, link)
		if err != nil {
			return err
		}
		notes, err = m.storePreview(ctx, link, fields)
	}
	if err != nil {
		return err
	}
	m.publish(ctx, notes)
	return nil
}
