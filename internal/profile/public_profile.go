package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

func (m Module) publicProfile(ctx context.Context, userID uuid.UUID) (*PublicProfile, error) {
	p, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	programmes, err := m.programmes.list(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := &PublicProfile{
		ID:           p.ID,
		Email:        p.Email,
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		UniversityID: p.UniversityID,
		AvatarURL:    m.avatarURL(ctx, p.Avatar),
		Bio:          p.Bio,
		Interests:    nonNil(p.Interests),
		Programmes:   make([]PublicProgramme, 0, len(programmes)),
	}
	for _, row := range programmes {
		out.Programmes = append(out.Programmes, PublicProgramme{
			Faculty:       &row.Faculty,
			Programme:     row.Programme,
			Level:         row.Level,
			ModeOfStudies: row.ModeOfStudies,
			YearOfStudy:   row.YearOfStudy,
			Status:        row.Status,
			IsPrimary:     row.IsPrimary,
		})
	}
	p.Privacy.apply(out)
	return out, nil
}

type profileQuery struct {
	UniversityID string
	Name         string
	Cursor       string
	Limit        int
}

func (m Module) listProfiles(ctx context.Context, q profileQuery) ([]ProfileSummary, *string, error) {
	filter := profileFilter{UniversityID: q.UniversityID, Name: q.Name, Limit: q.Limit + 1}
	if q.Cursor != "" {
		var after profileCursor
		if err := httpapi.DecodeCursor(q.Cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.profiles.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}

	var next *string
	if len(rows) > q.Limit {
		rows = rows[:q.Limit]
		last := rows[len(rows)-1]
		cursor, err := httpapi.EncodeCursor(profileCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		if err != nil {
			return nil, nil, err
		}
		next = &cursor
	}

	items := make([]ProfileSummary, 0, len(rows))
	for _, p := range rows {
		items = append(items, ProfileSummary{
			ID:           p.ID,
			FirstName:    p.FirstName,
			LastName:     p.LastName,
			UniversityID: p.UniversityID,
			AvatarURL:    m.avatarURL(ctx, p.Avatar),
		})
	}
	return items, next, nil
}
