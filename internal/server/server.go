// Package server lists the modules that make up the public HTTP API. cmd/api
// and cmd/openapi both register through it, so the committed spec always
// describes what the server serves.
package server

import (
	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Modules struct {
	Auth auth.Module
}

func New(db *pgxpool.Pool, cfg *config.Config) (Modules, error) {
	authModule, err := auth.New(db, cfg)
	if err != nil {
		return Modules{}, err
	}
	return Modules{Auth: authModule}, nil
}

func (m Modules) Register(api huma.API) {
	m.Auth.Register(api)
}
