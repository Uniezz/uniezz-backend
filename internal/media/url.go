package media

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const privateURLTTL = 10 * time.Minute

func (m Module) URL(ctx context.Context, mediaID uuid.UUID) (string, error) {
	item, err := m.repo.getReady(ctx, mediaID)
	if err != nil {
		return "", err
	}
	return m.urlOf(ctx, item)
}

func (m Module) URLs(ctx context.Context, mediaIDs []uuid.UUID) (map[uuid.UUID]string, error) {
	urls := make(map[uuid.UUID]string, len(mediaIDs))
	if len(mediaIDs) == 0 {
		return urls, nil
	}

	items, err := m.repo.listReady(ctx, mediaIDs)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		url, err := m.urlOf(ctx, &item)
		if err != nil {
			return nil, err
		}
		urls[item.ID] = url
	}
	return urls, nil
}

func (m Module) urlOf(ctx context.Context, item *mediaRow) (string, error) {
	if kindRules[item.Kind].public {
		return m.storage.publicURL(item.ObjectKey)
	}
	return m.storage.presignGet(ctx, item.ObjectKey)
}
