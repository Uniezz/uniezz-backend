package auth

import (
	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(r chi.Router, db *pgxpool.Pool, cfg *config.Config) {
	handler := newHandler(db)
	handler.registerRoutes(r, cfg)
}
