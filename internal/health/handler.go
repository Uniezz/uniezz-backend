package health

import (
	"net/http"
	"sync/atomic"

	"github.com/go-chi/chi/v5"
)

type Handler struct {
	isShuttingDown *atomic.Bool
}

func newHandler(isShuttingDown *atomic.Bool) *Handler {
	return &Handler{isShuttingDown: isShuttingDown}
}

func (h *Handler) registerRoutes(r chi.Router) {
	r.Get("/health", h.healthCheck)
}

func (h *Handler) healthCheck(w http.ResponseWriter, r *http.Request) {
	if h.isShuttingDown.Load() {
		http.Error(w, "Service is shutting down", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
