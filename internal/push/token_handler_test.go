package push

import (
	"net/http"
	"testing"
)

func TestEveryOperationRequiresASession(t *testing.T) {
	e := newTestEnv(t)

	tests := []struct {
		name string
		call func() int
	}{
		{"register token", func() int { return e.api.Put("/push/tokens/me", map[string]any{"token": newToken()}).Code }},
		{"delete token", func() int { return e.api.Delete("/push/tokens/me").Code }},
		{"get settings", func() int { return e.api.Get("/push/settings/me").Code }},
		{"update settings", func() int { return e.api.Patch("/push/settings/me", map[string]any{"enabled": false}).Code }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.call(); got != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", got, http.StatusUnauthorized)
			}
		})
	}
}

func TestPushTokens(t *testing.T) {
	e := newTestEnv(t)

	t.Run("a session holds one token and a new one replaces it", func(t *testing.T) {
		u := e.user(t, "pl")
		first, second := newToken(), newToken()
		e.register(t, u, first)
		e.register(t, u, first)
		e.register(t, u, second)

		tokens := e.tokensOf(t, u.id)
		if len(tokens) != 1 || tokens[second] != u.sessionID {
			t.Errorf("tokens = %v, want only the second one on the session", tokens)
		}
	})

	t.Run("a token another session held moves to the new session", func(t *testing.T) {
		old := e.user(t, "pl")
		token := newToken()
		e.register(t, old, token)

		other := e.user(t, "pl")
		e.register(t, other, token)
		if len(e.tokensOf(t, old.id)) != 0 {
			t.Errorf("old user still holds the token")
		}
		if e.tokensOf(t, other.id)[token] != other.sessionID {
			t.Errorf("token did not move to the new session")
		}
	})

	t.Run("a token that is not an Expo push token is rejected", func(t *testing.T) {
		u := e.user(t, "pl")
		expectError(t, e.api.Put("/push/tokens/me", u.bearer, map[string]any{"token": "not-a-token"}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("removing the token is idempotent and keeps other devices", func(t *testing.T) {
		u := e.user(t, "pl")
		phone := e.session(t, u.id)
		e.register(t, u, newToken())
		kept := newToken()
		e.register(t, phone, kept)

		for range 2 {
			if rec := e.api.Delete("/push/tokens/me", u.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
			}
		}
		tokens := e.tokensOf(t, u.id)
		if len(tokens) != 1 || tokens[kept] != phone.sessionID {
			t.Errorf("tokens = %v, want only the other device", tokens)
		}
	})
}

func TestPushSettings(t *testing.T) {
	e := newTestEnv(t)

	t.Run("notifications are on until the student turns them off", func(t *testing.T) {
		u := e.user(t, "pl")
		if s := decode[NotificationSettings](t, e.api.Get("/push/settings/me", u.bearer), http.StatusOK); !s.Enabled {
			t.Errorf("default = %+v, want enabled", s)
		}
		if s := decode[NotificationSettings](t, e.api.Patch("/push/settings/me", u.bearer, map[string]any{"enabled": false}), http.StatusOK); s.Enabled {
			t.Errorf("after turning off = %+v", s)
		}
		if s := decode[NotificationSettings](t, e.api.Patch("/push/settings/me", u.bearer, map[string]any{}), http.StatusOK); s.Enabled {
			t.Errorf("an empty patch changed the setting: %+v", s)
		}
		if s := decode[NotificationSettings](t, e.api.Get("/push/settings/me", u.bearer), http.StatusOK); s.Enabled {
			t.Errorf("read back = %+v, want disabled", s)
		}
	})
}
