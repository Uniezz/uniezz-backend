// Package auth provides authentication and session management for the application. It supports multiple login methods, including USOS login for universities. The module handles user authentication, session creation, and token exchange.
package auth

import (
	"fmt"
	"log"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/usos"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Register(api huma.API, db *pgxpool.Pool, cfg *config.Config) error {
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

	registerOperations(api, usosH, sessionH)
	return nil
}

func registerOperations(api huma.API, usosH *usosHandler, sessionH *sessionHandler) {
	huma.Register(api, usosStartOperation, usosH.start)
	huma.Register(api, usosCallbackOperation, usosH.callback)
	huma.Register(api, exchangeOperation, sessionH.exchange)
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
