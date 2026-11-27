package profile

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

func (m Module) publicProfile(ctx context.Context, viewerID, userID uuid.UUID) (*PublicProfile, error) {
	hidden, err := m.profiles.hidden(ctx, viewerID, userID)
	if err != nil {
		return nil, err
	}
	if hidden {
		return nil, ErrNotFound
	}
	p, err := m.profiles.get(ctx, userID)
	if err != nil {
		return nil, err
	}
	connectionPreferences, err := m.profiles.getConnectionPreferences(ctx, userID)
	if err != nil {
		return nil, err
	}
	programmes, err := m.programmes.list(ctx, userID)
	if err != nil {
		return nil, err
	}
	followersCount, followingCount, err := m.follows.counts(ctx, userID)
	if err != nil {
		return nil, err
	}
	followedByMe, err := m.follows.exists(ctx, viewerID, userID)
	if err != nil {
		return nil, err
	}

	var age *int
	if connectionPreferences.BirthDate != nil {
		birthDate := *connectionPreferences.BirthDate
		today := m.now().UTC()
		value := today.Year() - birthDate.Year()
		if today.Month() < birthDate.Month() ||
			(today.Month() == birthDate.Month() && today.Day() < birthDate.Day()) {
			value--
		}
		age = &value
	}

	photos := make([]string, 0)
	avatarURL := m.mediaURL(ctx, p.Avatar)
	if avatarURL != nil {
		photos = append(photos, *avatarURL)
	}

	refs, err := m.profiles.listProfilePhotos(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range refs {
		if photoURL := m.mediaURL(ctx, &refs[i].Ref); photoURL != nil {
			photos = append(photos, *photoURL)
		}
	}

	out := &PublicProfile{
		ID:             p.ID,
		Email:          p.Email,
		FirstName:      p.FirstName,
		LastName:       p.LastName,
		Gender:         p.Gender,
		Age:            age,
		UniversityID:   p.UniversityID,
		AvatarURL:      avatarURL,
		Photos:         photos,
		BannerURL:      m.mediaURL(ctx, p.Banner),
		Bio:            p.Bio,
		SongURL:        p.SongURL,
		Interests:      text.NonNil(p.Interests),
		Programmes:     make([]PublicProgramme, 0, len(programmes)),
		FollowersCount: followersCount,
		FollowingCount: followingCount,
		FollowedByMe:   followedByMe,
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

func (m Module) Photos(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	refs, err := m.profiles.getManyPhotos(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	photos := make(map[uuid.UUID][]string, len(userIDs))
	for _, id := range userIDs {
		urls := make([]string, 0, len(refs[id]))
		for i := range refs[id] {
			if url := m.mediaURL(ctx, &refs[id][i]); url != nil {
				urls = append(urls, *url)
			}
		}
		photos[id] = urls
	}
	return photos, nil
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
