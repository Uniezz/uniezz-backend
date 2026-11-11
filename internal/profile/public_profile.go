package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

func (m Module) publicProfile(ctx context.Context, viewerID, userID uuid.UUID) (*PublicProfile, error) {
	blocked, err := m.profiles.blocked(ctx, viewerID, userID)
	if err != nil {
		return nil, err
	}
	if blocked {
		return nil, ErrNotFound
	}
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
		Gender:       p.Gender,
		UniversityID: p.UniversityID,
		AvatarURL:    m.mediaURL(ctx, p.Avatar),
		Bio:          p.Bio,
		SongURL:      p.SongURL,
		Interests:    text.NonNil(p.Interests),
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
	ViewerID     uuid.UUID
	UniversityID string
	Search       string
	Cursor       string
	Limit        int
}

func (m Module) listProfiles(ctx context.Context, q profileQuery) ([]ProfileSummary, *string, error) {
	words, err := text.SearchWords(q.Search)
	if err != nil {
		return nil, nil, ErrInvalidInput
	}
	filter := profileFilter{ViewerID: q.ViewerID, UniversityID: q.UniversityID, Words: words, Limit: q.Limit + 1}
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

	rows, next, err := httpapi.Page(rows, q.Limit, func(last profileRow) any {
		return profileCursor{SearchRank: last.SearchRank, CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	items := make([]ProfileSummary, 0, len(rows))
	for _, p := range rows {
		items = append(items, m.summary(ctx, p))
	}
	return items, next, nil
}

func (m Module) Summaries(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]ProfileSummary, error) {
	rows, err := m.profiles.getMany(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	summaries := make(map[uuid.UUID]ProfileSummary, len(rows))
	for _, p := range rows {
		summaries[p.ID] = m.summary(ctx, p)
	}
	return summaries, nil
}

func (m Module) summary(ctx context.Context, p profileRow) ProfileSummary {
	return ProfileSummary{
		ID:           p.ID,
		FirstName:    p.FirstName,
		LastName:     p.LastName,
		Gender:       p.Gender,
		UniversityID: p.UniversityID,
		AvatarURL:    m.mediaURL(ctx, p.Avatar),
	}
}
