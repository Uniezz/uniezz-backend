package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// minOTPHashKeyLength keeps the HMAC key at least as long as the SHA-256 output.
const minOTPHashKeyLength = 32

const DefaultServiceName = "uniezz-api"

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
	OTPTestEmails      []string

	ServiceName       string
	SentryDSN         string
	SentryEnvironment string
	SentryRelease     string

	S3Region          string
	S3Bucket          string
	S3AccessKeyID     string
	S3SecretAccessKey string
	S3Endpoint        string
	S3PublicEndpoint  string
	S3ForcePathStyle  bool
	MediaPublicURL    string
}

func Load() (*Config, error) {
	s3ForcePathStyle, err := strconv.ParseBool(getEnv("S3_FORCE_PATH_STYLE", "false"))
	if err != nil {
		return nil, fmt.Errorf("invalid S3_FORCE_PATH_STYLE: %w", err)
	}

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
		ServiceName:        getEnv("OTEL_SERVICE_NAME", DefaultServiceName),
		SentryDSN:          os.Getenv("SENTRY_DSN"),
		SentryEnvironment:  os.Getenv("SENTRY_ENVIRONMENT"),
		SentryRelease:      os.Getenv("SENTRY_RELEASE"),
		S3Region:           getEnv("S3_REGION", "us-east-1"),
		S3Bucket:           getEnv("S3_BUCKET", "uniezz-media"),
		S3AccessKeyID:      os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretAccessKey:  os.Getenv("S3_SECRET_ACCESS_KEY"),
		S3Endpoint:         os.Getenv("S3_ENDPOINT"),
		S3PublicEndpoint:   os.Getenv("S3_PUBLIC_ENDPOINT"),
		S3ForcePathStyle:   s3ForcePathStyle,
		MediaPublicURL:     os.Getenv("MEDIA_PUBLIC_URL"),
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

	if cfg.S3AccessKeyID == "" || cfg.S3SecretAccessKey == "" {
		return nil, fmt.Errorf("S3_ACCESS_KEY_ID and S3_SECRET_ACCESS_KEY environment variables are required")
	}

	cfg.CORSAllowedOrigins = splitOrigins(getEnv("CORS_ALLOWED_ORIGINS", cfg.AppWebURL))

	cfg.OTPTestEmails = splitEmails(os.Getenv("OTP_TEST_EMAILS"))
	if cfg.Env == "production" && len(cfg.OTPTestEmails) > 0 {
		return nil, fmt.Errorf("OTP_TEST_EMAILS must not be set in production")
	}

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

func splitEmails(s string) []string {
	var emails []string
	for email := range strings.SplitSeq(s, ",") {
		email = strings.ToLower(strings.TrimSpace(email))
		if email != "" {
			emails = append(emails, email)
		}
	}
	return emails
}
