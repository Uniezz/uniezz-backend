package connections

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) HideCandidate(ctx context.Context, viewerID, userID uuid.UUID) error {
	return m.discovery.hide(ctx, viewerID, userID)
}
