package config

import (
	"fmt"
	"os"
	"strings"
)

// minOTPHashKeyLength keeps the HMAC key at least as long as the SHA-256 output.
const minOTPHashKeyLength = 32

type UsosCredentials struct {
	BaseURL        string
	ConsumerKey    string
	ConsumerSecret string
}

type Config struct {
	Port               string
	Env                string
	DatabaseURL        string
	BackendPublicURL   string
	AppWebURL          string
	AppMobileScheme    string
	UsosConfigs        map[string]UsosCredentials
	CORSAllowedOrigins []string
	ResendAPIKey       string
	MailFrom           string
	OTPEmailTemplateID string
	OTPHashKey         string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		Env:                getEnv("ENV", "development"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		BackendPublicURL:   getEnv("BACKEND_PUBLIC_URL", "http://localhost:8080"),
		AppWebURL:          getEnv("APP_WEB_URL", "http://localhost:3000"),
		AppMobileScheme:    getEnv("APP_MOBILE_SCHEME", "uniezz://auth/callback"),
		ResendAPIKey:       os.Getenv("RESEND_API_KEY"),
		MailFrom:           getEnv("MAIL_FROM", "Uniezz <no-reply@mail.uniezz.pl>"),
		OTPEmailTemplateID: os.Getenv("OTP_EMAIL_TEMPLATE_ID"),
		OTPHashKey:         os.Getenv("OTP_HASH_KEY"),
		UsosConfigs: map[string]UsosCredentials{
			"umcs": {
				BaseURL:        "https://apps.umcs.pl",
				ConsumerKey:    os.Getenv("UMCS_USOS_CONSUMER_KEY"),
				ConsumerSecret: os.Getenv("UMCS_USOS_CONSUMER_SECRET"),
			},
			"umlub": {
				BaseURL:        "https://usosapps.umlub.pl",
				ConsumerKey:    os.Getenv("UMLUB_USOS_CONSUMER_KEY"),
				ConsumerSecret: os.Getenv("UMLUB_USOS_CONSUMER_SECRET"),
			},
		},
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable is required")
	}

	if cfg.ResendAPIKey == "" {
		return nil, fmt.Errorf("RESEND_API_KEY environment variable is required")
	}

	if cfg.OTPEmailTemplateID == "" {
		return nil, fmt.Errorf("OTP_EMAIL_TEMPLATE_ID environment variable is required")
	}

	if len(cfg.OTPHashKey) < minOTPHashKeyLength {
		return nil, fmt.Errorf("OTP_HASH_KEY environment variable must be at least %d characters", minOTPHashKeyLength)
	}

	cfg.CORSAllowedOrigins = splitOrigins(getEnv("CORS_ALLOWED_ORIGINS", cfg.AppWebURL))

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}

	return defaultValue
}

func splitOrigins(s string) []string {
	var origins []string
	for origin := range strings.SplitSeq(s, ",") {
		origin = strings.TrimSuffix(strings.TrimSpace(origin), "/")
		if origin != "" {
			origins = append(origins, origin)
		}
	}
	return origins
}
