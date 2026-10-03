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

func newConfig(universityId UniversityID, cfg *config.Config) *oauth1.Config {
	usosEndpoint := oauth1.Endpoint{
		RequestTokenURL: "https://apps.umcs.pl/services/oauth/request_token",
		AuthorizeURL:    "https://apps.umcs.pl/services/oauth/authorize",
		AccessTokenURL:  "https://apps.umcs.pl/services/oauth/access_token",
	}

	return &oauth1.Config{
		ConsumerKey:    cfg.UsosConfigs[string(universityId)].ConsumerKey,
		ConsumerSecret: cfg.UsosConfigs[string(universityId)].ConsumerSecret,
		CallbackURL:    "http://localhost:8080/auth/callback",
		Endpoint:       usosEndpoint,
	}
}

func (h *Handler) registerRoutes(r chi.Router, cfg *config.Config) {
	r.Get("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		config := newConfig(UniversityUMCS, cfg)
		requestToken, requestSecret, err := config.RequestToken()
		if err != nil {
			http.Error(w, "Failed to get request token", http.StatusInternalServerError)
			return
		}

		authorizationURL, err := config.AuthorizationURL(requestToken)
		if err != nil {
			http.Error(w, "Failed to get authorization URL", http.StatusInternalServerError)
			return
		}

		_, err = h.db.Exec(r.Context(), "INSERT INTO oauth_states (requestToken, requestSecret) VALUES ($1, $2)", requestToken, requestSecret)
		if err != nil {
			http.Error(w, "Failed to store request token and secret", http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, authorizationURL.String(), http.StatusFound)
	})

	r.Get("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		requestToken, verifier, err := oauth1.ParseAuthorizationCallback(r)
		if err != nil {
			http.Error(w, "Failed to parse authorization callback", http.StatusInternalServerError)
			return
		}

		var requestSecret string
		err = h.db.QueryRow(r.Context(),
			"SELECT requestSecret FROM oauth_states WHERE requestToken=$1 AND expiresAt > NOW() LIMIT 1",
			requestToken,
		).Scan(&requestSecret)

		if err != nil {
			http.Error(w, "Failed to retrieve request secret", http.StatusInternalServerError)
			return
		}

		config := newConfig(UniversityUMCS, cfg)
		accessToken, accessSecret, err := config.AccessToken(requestToken, requestSecret, verifier)
		if err != nil {
			http.Error(w, "Failed to get access token", http.StatusInternalServerError)
			return
		}

		oauth1.NewToken(accessToken, accessSecret)
	})
}
