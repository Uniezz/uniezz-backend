package health

import (
	"context"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(r chi.Router, isShuttingDown *atomic.Bool, db *pgxpool.Pool,
	media interface {
		CheckBucket(context.Context) error
	},
) {
	handler := newHandler(isShuttingDown, db, media)
	handler.registerRoutes(r)
}
