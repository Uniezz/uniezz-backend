package profile

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestPhotos(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	withPhotos := e.otpUser(t)
	without := e.otpUser(t)

	avatar := insertMedia(ctx, e.pool, withPhotos.id, "pending")
	first := insertMedia(ctx, e.pool, withPhotos.id, "pending")
	second := insertMedia(ctx, e.pool, withPhotos.id, "pending")
	decode[Profile](t, e.api.Patch("/profile/me", withPhotos.bearer, map[string]any{
		"avatarMediaId": avatar.String(),
	}), http.StatusOK)
	for _, photo := range []uuid.UUID{first, second} {
		rec := e.api.Post("/profile/photos", withPhotos.bearer, map[string]any{"mediaId": photo.String()})
		if rec.Code != http.StatusCreated {
			t.Fatalf("adding photo: expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	photos, err := e.module.Photos(ctx, []uuid.UUID{withPhotos.id, without.id})
	if err != nil {
		t.Fatal(err)
	}

	got := photos[withPhotos.id]
	want := []string{avatarURL(avatar), avatarURL(first), avatarURL(second)}
	if len(got) != len(want) {
		t.Fatalf("photos = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("photos[%d] = %s, want %s", i, got[i], want[i])
		}
	}
	if none, ok := photos[without.id]; !ok || none == nil || len(none) != 0 {
		t.Errorf("student without photos = %#v, want empty list", none)
	}
}
