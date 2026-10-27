// Package server lists the modules that make up the public HTTP API. cmd/api
// and cmd/openapi both register through it, so the committed spec always
// describes what the server serves.
package server

import (
	"context"
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/mail"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Modules struct {
	Auth  auth.Module
	Media media.Module
}

func New(ctx context.Context, db *pgxpool.Pool, cfg *config.Config) (Modules, error) {
	mediaModule, err := media.New(ctx, db, cfg)
	if err != nil {
		return Modules{}, fmt.Errorf("initialize media: %w", err)
	}

	mailModule := mail.NewResendSender(cfg)
	authModule, err := auth.New(db, cfg, mailModule)
	if err != nil {
		return Modules{}, err
	}

	return Modules{Auth: authModule, Media: mediaModule}, nil
}

func (m Modules) Register(api huma.API) {
	m.Auth.Register(api)
}
