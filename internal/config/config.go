package config

import (
	"fmt"
	"os"
)

type UsosCredentials struct {
	BaseURL        string
	ConsumerKey    string
	ConsumerSecret string
}

type Config struct {
	Port             string
	Env              string
	DatabaseURL      string
	BackendPublicURL string
	AppWebURL        string
	AppMobileScheme  string
	UsosConfigs      map[string]UsosCredentials
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:             getEnv("PORT", "8080"),
		Env:              getEnv("ENV", "development"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		BackendPublicURL: getEnv("BACKEND_PUBLIC_URL", "http://localhost:8080"),
		AppWebURL:        getEnv("APP_WEB_URL", "http://localhost:3000"),
		AppMobileScheme:  getEnv("APP_MOBILE_SCHEME", "uniezz://auth/callback"),
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

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists && value != "" {
		return value
	}

	return defaultValue
}
