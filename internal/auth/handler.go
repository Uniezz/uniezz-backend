package auth

import (
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func newHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

func (h *Handler) registerRoutes(r chi.Router, cfg *config.Config) {
	r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {

		// http.Redirect(w, r, authorizationURL.String(), http.StatusFound)
	})

	r.Get("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		// requestToken, verifier, err := oauth1.ParseAuthorizationCallback(r)
		// if err != nil {
		// 	http.Error(w, "Failed to parse authorization callback", http.StatusInternalServerError)
		// 	return
		// }

		// var requestSecret string
		// err = h.db.QueryRow(r.Context(),
		// 	"SELECT requestSecret FROM oauth_states WHERE requestToken=$1 AND expiresAt > NOW() LIMIT 1",
		// 	requestToken,
		// ).Scan(&requestSecret)

		// if err != nil {
		// 	http.Error(w, "Failed to retrieve request secret", http.StatusInternalServerError)
		// 	return
		// }

	})
}
