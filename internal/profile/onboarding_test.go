package profile

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestCompleteOnboarding(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	newcomer := func(t *testing.T, lastName string) testUser {
		t.Helper()
		u := e.newUser(t, "umcs", "onboarding-"+uuid.NewString()+"@umcs.pl")
		if _, err := e.pool.Exec(ctx, "UPDATE users SET onboarded_at = NULL, last_name = $2 WHERE id = $1", u.id, lastName); err != nil {
			t.Fatal(err)
		}
		return u
	}

	t.Run("sets onboardedAt once and keeps it on a second call", func(t *testing.T) {
		u := newcomer(t, "Nowak")
		if p := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK); p.OnboardedAt != nil {
			t.Fatalf("onboardedAt = %v before onboarding, want null", p.OnboardedAt)
		}

		if rec := e.api.Put("/profile/onboarding", u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		first := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK).OnboardedAt
		if first == nil {
			t.Fatal("onboardedAt is still null")
		}

		if rec := e.api.Put("/profile/onboarding", u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204 on repeat, got %d", rec.Code)
		}
		second := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK).OnboardedAt
		if second == nil || !second.Equal(*first) {
			t.Errorf("onboardedAt changed from %v to %v", first, second)
		}
	})

	t.Run("the student list shows a student only after onboarding", func(t *testing.T) {
		viewer := e.otpUser(t)
		tag := "onb" + uuid.NewString()[:8]
		u := newcomer(t, tag)
		listed := func() bool {
			body := decode[struct {
				Items []ProfileSummary `json:"items"`
			}](t, e.api.Get("/profiles?"+url.Values{"q": {tag}}.Encode(), viewer.bearer), http.StatusOK)
			return len(body.Items) == 1 && body.Items[0].ID == u.id
		}

		if listed() {
			t.Fatal("listed before onboarding")
		}
		if rec := e.api.Put("/profile/onboarding", u.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", rec.Code)
		}
		if !listed() {
			t.Error("not listed after onboarding")
		}
	})

	t.Run("needs a session", func(t *testing.T) {
		expectError(t, e.api.Put("/profile/onboarding"), http.StatusUnauthorized, "unauthorized")
	})
}
