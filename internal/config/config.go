package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port               string
	Env                string
	DatabaseURL        string
	UmcsConsumerKey    string
	UmcsConsumerSecret string
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:               getEnv("PORT", "8080"),
		Env:                getEnv("ENV", "development"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		UmcsConsumerKey:    os.Getenv("UMCS_USOS_CONSUMER_KEY"),
		UmcsConsumerSecret: os.Getenv("UMCS_USOS_CONSUMER_SECRET"),
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
