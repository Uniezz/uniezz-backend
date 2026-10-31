// Package server lists the modules that make up the public HTTP API. cmd/api
// and cmd/openapi both register through it, so the committed spec always
// describes what the server serves.
package server

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/mail"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Modules struct {
	Auth    auth.Module
	Media   media.Module
	Profile profile.Module
	Jobs    *jobs.Runner
}

func New(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) (Modules, error) {
	runner := jobs.NewRunner(db, cfg.JobWorkers, cfg.JobPollInterval)
	mediaModule, err := media.New(ctx, db, cfg, runner)
	if err != nil {
		return Modules{}, fmt.Errorf("initialize media: %w", err)
	}

	profileModule, err := profile.New(db, mediaModule, runner)
	if err != nil {
		return Modules{}, err
	}

	mailModule := mail.NewResendSender(cfg)
	authModule, err := auth.New(db, cfg, mailModule, profileModule)
	if err != nil {
		return Modules{}, err
	}

	return Modules{Auth: authModule, Media: mediaModule, Profile: profileModule, Jobs: runner}, nil
}

func (m Modules) Register(api huma.API) {
	api.UseMiddleware(m.Auth.Guard(api))
	m.Auth.Register(api)
	m.Media.Register(api)
	m.Profile.Register(api)
}
