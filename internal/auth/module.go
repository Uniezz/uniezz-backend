// Package auth provides authentication and session management for the application. It supports multiple login methods, including USOS login for universities. The module handles user authentication, session creation, and token exchange.
package auth

import (
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/usos"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	logins *logins
	usos   *usosLogin
	otp    *otpLogin
	guard  *guard

	webRedirectURL    string
	mobileRedirectURL string
}

func New(pool *pgxpool.Pool, cfg *config.Config, sender otpSender, profiles ProfileInitializer) (Module, error) {
	clients, err := newUsosClients(cfg)
	if err != nil {
		return Module{}, err
	}

	db := database.New(pool)
	sessions := NewSessionRepository(db)
	logins := newLogins(db, NewUserRepository(db), sessions, newLoginCodeRepository(db), defaultSessionTTL)
	logins.profiles = profiles

	return Module{
		logins: logins,
		usos:   newUsosLogin(clients, newUsosStateRepository(db)),
		otp: newOTPLogin(
			newOTPRequestRepository(db),
			sender,
			ratelimit.New(pool, "otp-send", otpSendLimit, otpSendWindow),
			newOTPHasher([]byte(cfg.OTPHashKey)),
			cfg.OTPTestEmails,
		),
		guard:             &guard{sessions: sessions, sessionTTL: defaultSessionTTL, now: time.Now},
		webRedirectURL:    cfg.AppWebURL + "/auth/done",
		mobileRedirectURL: cfg.AppMobileScheme,
	}, nil
}

func (m Module) Register(api huma.API) {
	usos := &usosHandler{
		usos:              m.usos,
		logins:            m.logins,
		webRedirectURL:    m.webRedirectURL,
		mobileRedirectURL: m.mobileRedirectURL,
	}
	sessions := &sessionHandler{logins: m.logins}
	universities := &universitiesHandler{}
	otp := &otpHandler{otp: m.otp, logins: m.logins}

	huma.Register(api, usosStartOperation, usos.start)
	huma.Register(api, usosCallbackOperation, usos.callback)
	huma.Register(api, exchangeOperation, sessions.exchange)
	huma.Register(api, universitiesOperation, universities.list)
	huma.Register(api, otpRequestOperation, otp.request)
	huma.Register(api, otpVerifyOperation, otp.verify)
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
			return nil, fmt.Errorf("auth: missing USOS credentials for %s", u.ID)
		}

		clients[u.ID] = usos.NewClient(creds.BaseURL, creds.ConsumerKey, creds.ConsumerSecret, callbackURL)
	}

	return clients, nil
}
