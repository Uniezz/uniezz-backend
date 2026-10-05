package auth

import (
	"fmt"
	"log"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/usos"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(r chi.Router, db *pgxpool.Pool, cfg *config.Config) error {
	clients, err := newUsosClients(cfg)
	if err != nil {
		return err
	}

	logins := newLogins(NewUserRepository(db), NewSessionRepository(db), newLoginCodeRepository(db), defaultSessionTTL)

	usosH := &usosHandler{
		usos:              newUsosLogin(clients, newUsosStateRepository(db)),
		logins:            logins,
		webRedirectURL:    cfg.AppWebURL + "/auth/done",
		mobileRedirectURL: cfg.AppMobileScheme,
	}
	sessionH := &sessionHandler{logins: logins}

	r.Route("/auth", func(r chi.Router) {
		r.Get("/usos/start", usosH.start)
		r.Get("/usos/callback", usosH.callback)
		r.Post("/exchange", sessionH.exchange)
	})

	return nil
}

func newUsosClients(cfg *config.Config) (map[UniversityID]usosClient, error) {
	callbackURL := cfg.BackendPublicURL + "/auth/usos/callback"
	clients := make(map[UniversityID]usosClient)

	for _, u := range GetUniversities() {
		if u.AuthType != AuthTypeUSOS {
			continue
		}

		creds, ok := cfg.UsosConfigs[string(u.ID)]
		if !ok || creds.ConsumerKey == "" || creds.ConsumerSecret == "" {
			if cfg.Env == "development" {
				log.Printf("auth: USOS login for %s is disabled: no credentials", u.ID)
				continue
			}
			return nil, fmt.Errorf("auth: missing USOS credentials for %s", u.ID)
		}

		clients[u.ID] = usos.NewClient(creds.BaseURL, creds.ConsumerKey, creds.ConsumerSecret, callbackURL)
	}

	return clients, nil
}
