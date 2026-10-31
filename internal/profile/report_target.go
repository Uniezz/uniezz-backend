package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

// ReportTarget returns a copy of the student's names, bio and interests, kept
// with a report about them. Email and study programmes are left out.
func (m Module) ReportTarget(ctx context.Context, userID uuid.UUID) (map[string]any, error) {
	p, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"firstName":    p.FirstName,
		"lastName":     p.LastName,
		"universityId": p.UniversityID,
		"bio":          p.Bio,
		"interests":    text.NonNil(p.Interests),
	}, nil
}
