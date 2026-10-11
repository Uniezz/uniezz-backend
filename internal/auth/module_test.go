package auth

import (
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/config"
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

	t.Run("development disables a university without credentials", func(t *testing.T) {
		creds := fullUsosCredentials()
		delete(creds, "umlub")

		clients, err := newUsosClients(testConfig("development", creds))
		if err != nil {
			t.Fatalf("expected no error in development, got: %v", err)
		}
		if _, ok := clients[UniversityUMLub]; ok {
			t.Error("expected umlub to be disabled")
		}
		if _, ok := clients[UniversityUMCS]; !ok {
			t.Error("expected umcs to stay enabled")
		}
	})

	for _, tc := range []struct {
		name  string
		creds config.UsosCredentials
	}{
		{name: "empty key", creds: config.UsosCredentials{BaseURL: "https://usosapps.umlub.pl", ConsumerSecret: "secret"}},
		{name: "empty secret", creds: config.UsosCredentials{BaseURL: "https://usosapps.umlub.pl", ConsumerKey: "key"}},
	} {
		t.Run("development also disables a university with "+tc.name, func(t *testing.T) {
			creds := fullUsosCredentials()
			creds["umlub"] = tc.creds

			clients, err := newUsosClients(testConfig("development", creds))
			if err != nil {
				t.Fatalf("expected no error in development, got: %v", err)
			}
			if _, ok := clients[UniversityUMLub]; ok {
				t.Error("expected umlub to be disabled")
			}
		})
	}

	t.Run("outside development missing credentials fail startup", func(t *testing.T) {
		creds := fullUsosCredentials()
		delete(creds, "umcs")

		for _, env := range []string{"production", "staging"} {
			if _, err := newUsosClients(testConfig(env, creds)); err == nil {
				t.Errorf("%s: expected an error for missing umcs credentials, got nil", env)
			}
		}
	})
}
