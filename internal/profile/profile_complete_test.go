package profile

import (
	"context"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/media"
)

func TestProfileCompleteCoins(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	coins := func(t *testing.T, u testUser) int {
		t.Helper()
		return decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK).Coins
	}
	almostFull := func(t *testing.T, u testUser) map[string]any {
		t.Helper()
		return map[string]any{
			"firstName":     "Jan",
			"lastName":      "Kowalski",
			"gender":        "male",
			"avatarMediaId": insertMedia(ctx, e.pool, u.id, "pending").String(),
			"bannerMediaId": insertMediaOf(ctx, e.pool, u.id, media.KindBanner, "pending").String(),
			"bio":           "Second-year CS student",
			"interests":     []string{"climbing"},
		}
	}
	const song = "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT"

	t.Run("filling the last field pays 50 coins", func(t *testing.T) {
		u := e.otpUser(t)
		decode[Profile](t, e.api.Patch("/profile/me", u.bearer, almostFull(t, u)), http.StatusOK)
		if got := coins(t, u); got != 0 {
			t.Fatalf("coins before the last field = %d, want 0", got)
		}

		got := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"songUrl": song}), http.StatusOK)
		if got.Coins != 50 {
			t.Errorf("coins after the last field = %d, want 50", got.Coins)
		}
	})

	t.Run("clearing a field and filling it again pays nothing more", func(t *testing.T) {
		u := e.otpUser(t)
		body := almostFull(t, u)
		body["songUrl"] = song
		decode[Profile](t, e.api.Patch("/profile/me", u.bearer, body), http.StatusOK)

		decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"bio": ""}), http.StatusOK)
		decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"bio": "Back again"}), http.StatusOK)
		if got := coins(t, u); got != 50 {
			t.Errorf("coins = %d, want 50", got)
		}
	})

	t.Run("a profile missing one field pays nothing", func(t *testing.T) {
		u := e.otpUser(t)
		body := almostFull(t, u)
		body["songUrl"] = song
		delete(body, "bannerMediaId")
		decode[Profile](t, e.api.Patch("/profile/me", u.bearer, body), http.StatusOK)
		if got := coins(t, u); got != 0 {
			t.Errorf("coins = %d, want 0", got)
		}
	})
}
