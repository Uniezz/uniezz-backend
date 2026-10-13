package auth

import (
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/config"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2/humatest"
)

func testConfig(env string, usosConfigs map[string]config.UsosCredentials) *config.Config {
	return &config.Config{
		Env:              env,
		BackendPublicURL: "https://api.test",
		UsosConfigs:      usosConfigs,
	}
}

func fullUsosCredentials() map[string]config.UsosCredentials {
	return map[string]config.UsosCredentials{
		"umcs":  {BaseURL: "https://apps.umcs.pl", ConsumerKey: "key", ConsumerSecret: "secret"},
		"umlub": {BaseURL: "https://usosapps.umlub.pl", ConsumerKey: "key", ConsumerSecret: "secret"},
	}
}

func TestNewUsosClients(t *testing.T) {
	t.Run("builds a client for every USOS university and none for OTP ones", func(t *testing.T) {
		clients, err := newUsosClients(testConfig("production", fullUsosCredentials()))
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		for _, u := range GetUniversities() {
			_, ok := clients[u.ID]
			if want := u.AuthType == AuthTypeUSOS; ok != want {
				t.Errorf("%s (%s): expected client present = %v, got %v", u.ID, u.AuthType, want, ok)
			}
		}
	})

	for _, tc := range []struct {
		name  string
		creds func(map[string]config.UsosCredentials)
	}{
		{name: "missing", creds: func(c map[string]config.UsosCredentials) { delete(c, "umlub") }},
		{name: "empty key", creds: func(c map[string]config.UsosCredentials) {
			c["umlub"] = config.UsosCredentials{BaseURL: "https://usosapps.umlub.pl", ConsumerSecret: "secret"}
		}},
		{name: "empty secret", creds: func(c map[string]config.UsosCredentials) {
			c["umlub"] = config.UsosCredentials{BaseURL: "https://usosapps.umlub.pl", ConsumerKey: "key"}
		}},
	} {
		t.Run(tc.name+" credentials fail startup in every environment", func(t *testing.T) {
			creds := fullUsosCredentials()
			tc.creds(creds)

			for _, env := range []string{"development", "staging", "production"} {
				if _, err := newUsosClients(testConfig(env, creds)); err == nil {
					t.Errorf("%s: expected an error for %s umlub credentials, got nil", env, tc.name)
				}
			}
		})
	}
}

func TestModule_ZeroValueDescribesOperations(t *testing.T) {
	// cmd/openapi registers a zero Module: no database, no USOS credentials.
	_, api := humatest.New(t, httpapi.NewConfig(&config.Config{}))
	Module{}.Register(api)

	for _, want := range []struct{ path, method string }{
		{"/auth/usos/start", http.MethodGet},
		{"/auth/usos/callback", http.MethodGet},
		{"/auth/exchange", http.MethodPost},
	} {
		item := api.OpenAPI().Paths[want.path]
		if item == nil {
			t.Errorf("%s is missing from the spec", want.path)
			continue
		}
		if op := item.Get; want.method == http.MethodGet && op == nil {
			t.Errorf("GET %s is missing from the spec", want.path)
		}
		if op := item.Post; want.method == http.MethodPost && op == nil {
			t.Errorf("POST %s is missing from the spec", want.path)
		}
	}
}

func TestNew(t *testing.T) {
	if _, err := New(nil, testConfig("production", fullUsosCredentials())); err != nil {
		t.Fatalf("expected no error with full credentials, got: %v", err)
	}

	creds := fullUsosCredentials()
	delete(creds, "umcs")
	if _, err := New(nil, testConfig("production", creds)); err == nil {
		t.Error("expected an error for missing umcs credentials outside development, got nil")
	}
}
