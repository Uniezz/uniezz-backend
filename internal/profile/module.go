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
	repo  *repository
	media mediaService
	jobs  *jobs.Runner
}

func New(db *pgxpool.Pool, mediaModule mediaService, runner *jobs.Runner) (Module, error) {
	m := Module{repo: newRepository(db), media: mediaModule, jobs: runner}

	err := jobs.Register(runner, ImportAvatar, m.importAvatar, jobs.Options{Timeout: 30 * time.Second, MaxAttempts: 5})
	if err != nil {
		return Module{}, fmt.Errorf("profile: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	huma.Register(api, getMeOperation, m.getMe)
	huma.Register(api, updateMeOperation, m.updateMe)
	huma.Register(api, deleteAvatarOperation, m.deleteAvatar)
	huma.Register(api, listProgrammesOperation, m.listProgrammes)
	huma.Register(api, createProgrammeOperation, m.createProgramme)
	huma.Register(api, replaceProgrammeOperation, m.replaceProgramme)
	huma.Register(api, deleteProgrammeOperation, m.deleteProgramme)
}
