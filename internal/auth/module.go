// Package auth provides authentication and session management for the application. It supports multiple login methods, including USOS login for universities. The module handles user authentication, session creation, and token exchange.
package auth

import (
	"fmt"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/usos"
	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Module struct {
	usos         *usosHandler
	session      *sessionHandler
	universities *universitiesHandler
	otp          *otpHandler
	me           *meHandler
}

func New(db *pgxpool.Pool, cfg *config.Config, sender otpSender) (Module, error) {
	clients, err := newUsosClients(cfg)
	if err != nil {
		return Module{}, err
	}

	logins := newLogins(NewUserRepository(db), NewSessionRepository(db), newLoginCodeRepository(db), defaultSessionTTL)

	return Module{
		usos: &usosHandler{
			usos:              newUsosLogin(clients, newUsosStateRepository(db)),
			logins:            logins,
			webRedirectURL:    cfg.AppWebURL + "/auth/done",
			mobileRedirectURL: cfg.AppMobileScheme,
		},
		session:      &sessionHandler{logins: logins},
		universities: &universitiesHandler{},
		me:           &meHandler{logins: logins},
		otp: &otpHandler{
			otp: newOTPLogin(
				newOTPRequestRepository(db),
				sender,
				ratelimit.New(db, "otp-send", otpSendLimit, otpSendWindow),
				newOTPHasher([]byte(cfg.OTPHashKey)),
				cfg.OTPTestEmails,
			),
			logins: logins,
		},
	}, nil
}

func (m Module) Register(api huma.API) {
	huma.Register(api, usosStartOperation, m.usos.start)
	huma.Register(api, usosCallbackOperation, m.usos.callback)
	huma.Register(api, exchangeOperation, m.session.exchange)
	huma.Register(api, universitiesOperation, m.universities.list)
	huma.Register(api, otpRequestOperation, m.otp.request)
	huma.Register(api, otpVerifyOperation, m.otp.verify)
	huma.Register(api, meOperation, m.me.getMe)
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
