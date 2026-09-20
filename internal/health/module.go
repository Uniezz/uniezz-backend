package health

import (
	"sync/atomic"

	"github.com/go-chi/chi/v5"
)

func Register(r chi.Router, isShuttingDown *atomic.Bool) {
	handler := newHandler(isShuttingDown)
	handler.registerRoutes(r)
}
