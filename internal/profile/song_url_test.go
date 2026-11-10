package profile

import (
	"net/http"
	"strings"
	"testing"
)

func TestSongURL(t *testing.T) {
	e := newTestEnv(t)
	u := e.otpUser(t)
	viewer := e.otpUser(t)
	link := "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT"

	body := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)
	if body.SongURL != nil {
		t.Fatalf("songUrl = %v, want null", body.SongURL)
	}

	body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"songUrl": "  " + link + "  ",
	}), http.StatusOK)
	if body.SongURL == nil || *body.SongURL != link {
		t.Fatalf("songUrl = %v, want the trimmed link", body.SongURL)
	}

	body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"bio": "Still here",
	}), http.StatusOK)
	if body.SongURL == nil || *body.SongURL != link {
		t.Errorf("omitting songUrl changed it to %v", body.SongURL)
	}

	public := decode[PublicProfile](t, e.api.Get("/profiles/"+u.id.String(), viewer.bearer), http.StatusOK)
	if public.SongURL == nil || *public.SongURL != link {
		t.Errorf("public songUrl = %v", public.SongURL)
	}

	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"songUrl": "ftp://example.com/a"}), http.StatusBadRequest, "invalid_input")
	body = decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)
	if body.SongURL == nil || *body.SongURL != link {
		t.Errorf("a rejected link replaced the saved one: %v", body.SongURL)
	}

	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"songUrl": "https://example.com/" + strings.Repeat("a", 500),
	}), http.StatusBadRequest, "invalid_input")

	body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"songUrl": ""}), http.StatusOK)
	if body.SongURL != nil {
		t.Errorf("an empty songUrl must clear it, got %v", body.SongURL)
	}
	public = decode[PublicProfile](t, e.api.Get("/profiles/"+u.id.String(), viewer.bearer), http.StatusOK)
	if public.SongURL != nil {
		t.Errorf("public songUrl = %v, want null", public.SongURL)
	}
}
