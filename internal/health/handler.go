package health

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	isShuttingDown *atomic.Bool
	db             *pgxpool.Pool
}

func newHandler(isShuttingDown *atomic.Bool, db *pgxpool.Pool) *Handler {
	return &Handler{isShuttingDown: isShuttingDown, db: db}
}

func (h *Handler) registerRoutes(r chi.Router) {
	r.Get("/healthz/live", h.healthLiveCheck)
	r.Get("/healthz/ready", h.healthReadyCheck)
}

func (h *Handler) healthLiveCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

func (h *Handler) healthReadyCheck(w http.ResponseWriter, r *http.Request) {
	if h.isShuttingDown.Load() {
		http.Error(w, "Service is shutting down", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		http.Error(w, "Database connection failed", http.StatusServiceUnavailable)
		return
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
