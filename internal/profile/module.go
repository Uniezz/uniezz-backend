// Package profile owns what a student shows about themselves: names, avatar,
// bio, interests, study programmes and who may see them. Every field is editable by the student; USOS only prefills
// the profile when the account is created. Deleting the profile deletes the whole account.
package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/feed"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/study"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	resumeDeletionsEvery   = time.Hour
	deleteAccountTimeout   = 5 * time.Minute
	deleteAccountAttempts  = 10
	resumeDeletionsTimeout = time.Minute
)

var (
	_ mediaService = media.Module{}
	_ forgetter    = feed.Module{}
	_ forgetter    = study.Module{}
)

type mediaService interface {
	Claim(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	URLOf(ctx context.Context, ref media.Ref) (string, error)
	MarkDeleting(ctx context.Context, mediaID uuid.UUID) error
	Delete(ctx context.Context, mediaID uuid.UUID) error
	Import(ctx context.Context, ownerUserID uuid.UUID, kind media.Kind, sourceURL string) (uuid.UUID, error)
	ForgetUser(ctx context.Context, userID uuid.UUID) error
}

type forgetter interface {
	ForgetUser(ctx context.Context, userID uuid.UUID) error
}

type Module struct {
	tx         database.Transactor
	profiles   *profileRepository
	programmes *programmeRepository
	media      mediaService
	feed       forgetter
	study      forgetter
	jobs       *jobs.Runner
	now        func() time.Time
}

func New(pool *pgxpool.Pool, mediaModule mediaService, feedModule, studyModule forgetter, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{
		tx:         db,
		profiles:   newProfileRepository(db),
		programmes: newProgrammeRepository(db),
		media:      mediaModule,
		feed:       feedModule,
		study:      studyModule,
		jobs:       runner,
		now:        time.Now,
	}

	if err := jobs.Register(runner, m.importAvatar, jobs.Options{Timeout: 30 * time.Second, MaxAttempts: 5}); err != nil {
		return Module{}, fmt.Errorf("profile: %w", err)
	}
	if err := jobs.Register(runner, m.deleteAccountTask, jobs.Options{Timeout: deleteAccountTimeout, MaxAttempts: deleteAccountAttempts}); err != nil {
		return Module{}, fmt.Errorf("profile: %w", err)
	}
	if err := jobs.Register(runner, m.resumeDeletions, jobs.Options{Timeout: resumeDeletionsTimeout, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("profile: %w", err)
	}
	if err := jobs.Schedule(runner, "profile-resume-deletions", ResumeDeletionsArgs{}, resumeDeletionsEvery); err != nil {
		return Module{}, fmt.Errorf("profile: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	profiles := &profileHandler{profile: m}
	programmes := &programmeHandler{programmes: m.programmes}
	public := &publicProfileHandler{profile: m}

	huma.Register(api, getMeOperation, profiles.getMe)
	huma.Register(api, updateMeOperation, profiles.updateMe)
	huma.Register(api, deleteMeOperation, profiles.deleteMe)
	huma.Register(api, deleteAvatarOperation, profiles.deleteAvatar)
	huma.Register(api, deleteBannerOperation, profiles.deleteBanner)
	huma.Register(api, listProgrammesOperation, programmes.listProgrammes)
	huma.Register(api, createProgrammeOperation, programmes.createProgramme)
	huma.Register(api, replaceProgrammeOperation, programmes.replaceProgramme)
	huma.Register(api, deleteProgrammeOperation, programmes.deleteProgramme)
	huma.Register(api, listProfilesOperation, public.listProfiles)
	huma.Register(api, getProfileOperation, public.getProfile)
}
