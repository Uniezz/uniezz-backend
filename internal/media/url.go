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

	if kindRules[item.Kind].public {
		return m.storage.publicURL(item.ObjectKey)
	}
	return m.storage.presignGet(ctx, item.ObjectKey)
}
