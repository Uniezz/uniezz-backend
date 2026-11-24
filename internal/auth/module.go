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
	tx           database.Transactor
	universities *universityRepository
	logins       *logins
	usos         *usosLogin
	otp          *otpLogin
	guard        *guard
	roles        *roleRepository
	bans         *banRepository

	webRedirectURL    string
	mobileRedirectURL string
}

func New(pool *pgxpool.Pool, cfg *config.Config, sender otpSender, profiles ProfileInitializer, referrals ReferralRewarder) (Module, error) {
	clients, err := newUsosClients(cfg)
	if err != nil {
		return Module{}, err
	}

	db := database.New(pool)
	universities := newUniversityRepository(db)
	sessions := NewSessionRepository(db)
	logins := newLogins(db, NewUserRepository(db), sessions, newLoginCodeRepository(db), universities, defaultSessionTTL)
	logins.profiles = profiles
	logins.referrals = referrals

	return Module{
		tx:           db,
		universities: universities,
		logins:       logins,
		usos:         newUsosLogin(clients, newUsosStateRepository(db), universities),
		otp: newOTPLogin(
			newOTPRequestRepository(db),
			universities,
			sender,
			ratelimit.New(pool, "otp-send", otpSendLimit, otpSendWindow),
			newOTPHasher([]byte(cfg.OTPHashKey)),
			cfg.OTPTestEmails,
		),
		guard:             &guard{sessions: sessions, sessionTTL: defaultSessionTTL, now: time.Now},
		roles:             newRoleRepository(db),
		bans:              newBanRepository(db),
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
	universities := &universitiesHandler{auth: m}
	otp := &otpHandler{otp: m.otp, logins: m.logins}
	invites := &inviteCodeHandler{auth: m}

	huma.Register(api, usosStartOperation, usos.start)
	huma.Register(api, usosCallbackOperation, usos.callback)
	huma.Register(api, exchangeOperation, sessions.exchange)
	huma.Register(api, universitiesOperation, universities.getUniversities)
	huma.Register(api, getUniversityOperation, universities.getUniversity)
	huma.Register(api, createUniversityOperation, universities.createUniversity)
	huma.Register(api, replaceUniversityOperation, universities.replaceUniversity)
	huma.Register(api, otpRequestOperation, otp.request)
	huma.Register(api, otpVerifyOperation, otp.verify)
	huma.Register(api, getMyInviteCodeOperation, invites.getMyInviteCode)
	huma.Register(api, redeemInviteOperation, invites.redeemInvite)
}

func newUsosClients(cfg *config.Config) (map[UniversityID]usosClient, error) {
	callbackURL := cfg.BackendPublicURL + "/auth/usos/callback"
	clients := make(map[UniversityID]usosClient)

	for id, creds := range cfg.UsosConfigs {
		if creds.ConsumerKey == "" || creds.ConsumerSecret == "" {
			return nil, fmt.Errorf("auth: missing USOS credentials for %s", id)
		}
		clients[UniversityID(id)] = usos.NewClient(creds.BaseURL, creds.ConsumerKey, creds.ConsumerSecret, callbackURL)
	}

	return clients, nil
}
