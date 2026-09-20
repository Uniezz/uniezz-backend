package health

import (
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(r chi.Router, isShuttingDown *atomic.Bool, db *pgxpool.Pool) {
	handler := newHandler(isShuttingDown, db)
	handler.registerRoutes(r)
}
