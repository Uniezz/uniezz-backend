package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func productionConfig(usos map[string]config.UsosCredentials) *config.Config {
	return &config.Config{
		Env:               "production",
		BackendPublicURL:  "https://api.test",
		UsosConfigs:       usos,
		S3Region:          "us-east-1",
		S3Bucket:          "test-bucket",
		S3Endpoint:        "http://127.0.0.1:1",
		S3AccessKeyID:     "test-access-key",
		S3SecretAccessKey: "test-secret-key",
	}
}

func TestNew(t *testing.T) {
	full := map[string]config.UsosCredentials{
		"umcs":  {BaseURL: "https://apps.umcs.pl", ConsumerKey: "key", ConsumerSecret: "secret"},
		"umlub": {BaseURL: "https://usosapps.umlub.pl", ConsumerKey: "key", ConsumerSecret: "secret"},
	}

	t.Run("builds every module with full config", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		if _, err := New(ctx, nil, productionConfig(full)); err != nil {
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

func TestModules_OnlyAuthRoutesArePublic(t *testing.T) {
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	Modules{}.Register(api)

	for path, item := range api.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Get, item.Post, item.Put, item.Patch, item.Delete} {
			if op == nil {
				continue
			}
			public := strings.HasPrefix(path, "/auth/")
			if hasBearer := len(op.Security) > 0; hasBearer == public {
				t.Errorf("%s %s: public=%v but security=%v", op.Method, path, public, op.Security)
			}
		}
	}
}
