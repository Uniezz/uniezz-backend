package connections

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

func (m Module) ReportTarget(ctx context.Context, viewerID, candidateID uuid.UUID) (uuid.UUID, map[string]any, error) {
	userID, err := m.discovery.lookup(ctx, viewerID, candidateID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	s, err := m.discovery.snapshot(ctx, userID)
	if err != nil {
		return uuid.Nil, nil, err
	}
	return userID, map[string]any{"intent": s.Intent, "bio": s.Bio, "interests": text.NonNil(s.Interests)}, nil
}

func (m Module) HideCandidate(ctx context.Context, viewerID, userID uuid.UUID) error {
	return m.discovery.hide(ctx, viewerID, userID)
}
