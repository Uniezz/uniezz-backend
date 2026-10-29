// Package profile owns what a student shows about themselves: names, avatar and
// study programmes. Every field is editable by the student; USOS only prefills
// the profile when the account is created.
package profile

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var _ mediaService = media.Module{}

type mediaService interface {
	Claim(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	URL(ctx context.Context, mediaID uuid.UUID) (string, error)
	MarkDeleting(ctx context.Context, tx pgx.Tx, mediaID uuid.UUID) error
	Delete(ctx context.Context, mediaID uuid.UUID) error
	Import(ctx context.Context, ownerUserID uuid.UUID, kind media.Kind, sourceURL string) (uuid.UUID, error)
}

type Module struct {
	profiles   *profileRepository
	programmes *programmeRepository
	media      mediaService
	jobs       *jobs.Runner

	profileHandler   *profileHandler
	programmeHandler *programmeHandler
}

func New(db *pgxpool.Pool, mediaModule mediaService, runner *jobs.Runner) (Module, error) {
	m := Module{
		profiles:   newProfileRepository(db),
		programmes: newProgrammeRepository(db),
		media:      mediaModule,
		jobs:       runner,
	}

	err := jobs.Register(runner, ImportAvatar, m.importAvatar, jobs.Options{Timeout: 30 * time.Second, MaxAttempts: 5})
	if err != nil {
		return Module{}, fmt.Errorf("profile: %w", err)
	}

	m.profileHandler = &profileHandler{profile: m}
	m.programmeHandler = &programmeHandler{programmes: m.programmes}
	return m, nil
}

func (m Module) Register(api huma.API) {
	huma.Register(api, getMeOperation, m.profileHandler.getMe)
	huma.Register(api, updateMeOperation, m.profileHandler.updateMe)
	huma.Register(api, deleteAvatarOperation, m.profileHandler.deleteAvatar)
	huma.Register(api, listProgrammesOperation, m.programmeHandler.listProgrammes)
	huma.Register(api, createProgrammeOperation, m.programmeHandler.createProgramme)
	huma.Register(api, replaceProgrammeOperation, m.programmeHandler.replaceProgramme)
	huma.Register(api, deleteProgrammeOperation, m.programmeHandler.deleteProgramme)
}
