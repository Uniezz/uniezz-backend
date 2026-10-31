package media

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const privateURLTTL = 10 * time.Minute

type Ref struct {
	Kind      Kind
	ObjectKey string
}

func (m Module) URL(ctx context.Context, mediaID uuid.UUID) (string, error) {
	item, err := m.repo.getReady(ctx, mediaID)
	if err != nil {
		return "", err
	}
	return m.URLOf(ctx, Ref{Kind: item.Kind, ObjectKey: item.ObjectKey})
}

func (m Module) URLOf(ctx context.Context, ref Ref) (string, error) {
	if kindRules[ref.Kind].public {
		return m.storage.publicURL(ref.ObjectKey)
	}
	return m.storage.presignGet(ctx, ref.ObjectKey)
}
