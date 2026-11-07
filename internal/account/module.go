// Package account lets a student delete their account. It starts the deletion
// in auth and runs the cleanup as a background job, asking each module to
// forget the user before the user row goes.
package account

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/feed"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/moderation"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	_ accounts  = auth.Module{}
	_ forgetter = feed.Module{}
	_ forgetter = media.Module{}
	_ forgetter = moderation.Module{}
)

type accounts interface {
	BeginDeletion(ctx context.Context, userID uuid.UUID) (bool, error)
	FinishDeletion(ctx context.Context, userID uuid.UUID) error
	StuckDeletions(ctx context.Context, olderThan time.Duration, limit int) ([]uuid.UUID, error)
}

type forgetter interface {
	ForgetUser(ctx context.Context, userID uuid.UUID) error
}

type Module struct {
	tx         database.Transactor
	jobs       *jobs.Runner
	now        func() time.Time
	accounts   accounts
	feed       forgetter
	media      forgetter
	moderation forgetter
}

func New(pool *pgxpool.Pool, authModule accounts, feedModule, mediaModule, moderationModule forgetter, runner *jobs.Runner) (Module, error) {
	m := Module{
		tx:         database.New(pool),
		jobs:       runner,
		now:        time.Now,
		accounts:   authModule,
		feed:       feedModule,
		media:      mediaModule,
		moderation: moderationModule,
	}

	if err := jobs.Register(runner, m.deleteAccount, jobs.Options{Timeout: 5 * time.Minute, MaxAttempts: 10}); err != nil {
		return Module{}, fmt.Errorf("account: %w", err)
	}
	if err := jobs.Register(runner, m.resumeDeletions, jobs.Options{Timeout: 10 * time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("account: %w", err)
	}
	if err := jobs.Schedule(runner, ResumeDeletionsArgs{}, time.Hour); err != nil {
		return Module{}, fmt.Errorf("account: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	accounts := &accountHandler{account: m}
	huma.Register(api, deleteAccountOperation, accounts.deleteAccount)
}
