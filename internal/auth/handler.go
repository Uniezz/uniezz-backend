package auth

import (
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/dghubble/oauth1"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	db *pgxpool.Pool
}

func newHandler(db *pgxpool.Pool) *Handler {
	return &Handler{db: db}
}

var usosEndpoint = oauth1.Endpoint{
	RequestTokenURL: "https://apps.umcs.pl/services/oauth/request_token",
	AuthorizeURL:    "https://apps.umcs.pl/services/oauth/authorize",
	AccessTokenURL:  "https://apps.umcs.pl/services/oauth/access_token",
}

func (h *Handler) registerRoutes(r chi.Router, cfg *config.Config) {
	r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		config := oauth1.Config{
			ConsumerKey:    cfg.UmcsConsumerKey,
			ConsumerSecret: cfg.UmcsConsumerSecret,
			CallbackURL:    "http://localhost:8080/auth/callback",
			Endpoint:       usosEndpoint,
		}

		requestToken, requestSecret, err := config.RequestToken()
		if err != nil {
			http.Error(w, "Failed to get request token", http.StatusInternalServerError)
		}

		authorizationURL, err := config.AuthorizationURL(requestToken)
		if err != nil {
			http.Error(w, "Failed to get authorization URL", http.StatusInternalServerError)
		}

		_, err = h.db.Exec(r.Context(), "INSERT INTO oauth_states (requestToken, requestSecret) VALUES ($1, $2)", requestToken, requestSecret)
		if err != nil {
			http.Error(w, "Failed to store request token and secret", http.StatusInternalServerError)
		}

		http.Redirect(w, r, authorizationURL.String(), http.StatusFound)
	})

	r.Get("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		_, _, err := oauth1.ParseAuthorizationCallback(r)
		if err != nil {
			http.Error(w, "Failed to parse authorization callback", http.StatusInternalServerError)
		}
	})
}
