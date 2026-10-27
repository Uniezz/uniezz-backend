package server

import (
	"context"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func productionConfig(usos map[string]config.UsosCredentials) *config.Config {
	return &config.Config{
		Env:                "production",
		BackendPublicURL:   "https://api.test",
		UsosConfigs:        usos,
		AWSRegion:          "us-east-1",
		AWSS3Bucket:        "test-bucket",
		AWSAccessKeyID:     "test-access-key",
		AWSSecretAccessKey: "test-secret-key",
	}
}

func TestNew(t *testing.T) {
	full := map[string]config.UsosCredentials{
		"umcs":  {BaseURL: "https://apps.umcs.pl", ConsumerKey: "key", ConsumerSecret: "secret"},
		"umlub": {BaseURL: "https://usosapps.umlub.pl", ConsumerKey: "key", ConsumerSecret: "secret"},
	}

	t.Run("builds every module with full config", func(t *testing.T) {
		if _, err := New(context.Background(), nil, productionConfig(full)); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("returns the module error instead of panicking", func(t *testing.T) {
		if _, err := New(context.Background(), nil, productionConfig(nil)); err == nil {
			t.Error("expected an error for missing USOS credentials in production, got nil")
		}
	})
}

func TestModules_ZeroValueRegistersEveryOperation(t *testing.T) {
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	Modules{}.Register(api)

	for _, path := range []string{"/auth/usos/start", "/auth/usos/callback", "/auth/exchange"} {
		if api.OpenAPI().Paths[path] == nil {
			t.Errorf("%s is missing from the spec", path)
		}
	}
}
