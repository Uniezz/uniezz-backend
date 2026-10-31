package profile

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestGetMe(t *testing.T) {
	e := newTestEnv(t)

	t.Run("has the same keys for every account, with an empty programme list", func(t *testing.T) {
		u := e.otpUser(t)

		body := decode[map[string]any](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)

		var keys []string
		for k := range body {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		want := []string{"avatarUrl", "createdAt", "email", "firstName", "id", "lastName", "programmes", "universityId"}
		if !slices.Equal(keys, want) {
			t.Errorf("expected keys %v, got %v", want, keys)
		}
		if body["id"] != u.id.String() || body["universityId"] != "kul" || body["firstName"] != "Jan" {
			t.Errorf("unexpected profile %v", body)
		}
		if programmes, ok := body["programmes"].([]any); !ok || len(programmes) != 0 {
			t.Errorf("expected programmes [], got %v", body["programmes"])
		}
		if body["avatarUrl"] != nil {
			t.Errorf("expected avatarUrl null, got %v", body["avatarUrl"])
		}
	})

	t.Run("lists every programme, primary first, and the avatar URL", func(t *testing.T) {
		u := e.newUser(t, "umcs", "profile-"+uuid.NewString()+"@umcs.pl")
		ctx := context.Background()

		err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
			_, err := e.module.InitProfile(ctx, tx, u.id, auth.InitialProfile{Programmes: []auth.InitialProgramme{
				{Faculty: "Wydział Prawa", Programme: "Prawo", Level: "long_cycle"},
				{Faculty: "Wydział Matematyki, Fizyki i Informatyki", Programme: "Informatyka", ModeOfStudies: "full_time", IsPrimary: true},
			}})
			return err
		})
		if err != nil {
			t.Fatalf("InitProfile failed: %v", err)
		}
		avatarID := insertMedia(ctx, e.pool, u.id, "ready")
		if _, err := e.pool.Exec(ctx, "UPDATE users SET avatar_media_id = $2 WHERE id = $1", u.id, avatarID); err != nil {
			t.Fatal(err)
		}

		body := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)

		if len(body.Programmes) != 2 || body.Programmes[0].Programme != "Informatyka" || !body.Programmes[0].IsPrimary {
			t.Fatalf("expected two programmes with Informatyka primary first, got %+v", body.Programmes)
		}
		second := body.Programmes[1]
		if second.Faculty != "Wydział Prawa" || second.Level == nil || *second.Level != "long_cycle" || second.Status != "active" || second.YearOfStudy != nil {
			t.Errorf("unexpected second programme %+v", second)
		}
		if body.AvatarURL == nil || *body.AvatarURL != "https://media.test/"+avatarID.String() {
			t.Errorf("expected the avatar URL, got %v", body.AvatarURL)
		}
	})

	t.Run("needs a session", func(t *testing.T) {
		expectError(t, e.api.Get("/profile/me"), http.StatusUnauthorized, "unauthorized")
	})
}

func TestUpdateMe(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("changes only the names that are sent, trimmed", func(t *testing.T) {
		u := e.otpUser(t)

		body := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"firstName": "  Anna "}), http.StatusOK)

		if *body.FirstName != "Anna" || *body.LastName != "Kowalski" {
			t.Errorf("expected Anna Kowalski, got %q %q", *body.FirstName, *body.LastName)
		}
	})

	t.Run("a blank name is a 400", func(t *testing.T) {
		u := e.otpUser(t)
		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"lastName": "   "}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("sets an uploaded avatar, then replaces it and deletes the old one", func(t *testing.T) {
		u := e.otpUser(t)
		first := insertMedia(ctx, e.pool, u.id, "pending")
		second := insertMedia(ctx, e.pool, u.id, "pending")

		body := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": first.String()}), http.StatusOK)
		if body.AvatarURL == nil || *body.AvatarURL != "https://media.test/"+first.String() {
			t.Fatalf("expected the first avatar, got %v", body.AvatarURL)
		}

		body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": second.String()}), http.StatusOK)
		if body.AvatarURL == nil || *body.AvatarURL != "https://media.test/"+second.String() {
			t.Fatalf("expected the second avatar, got %v", body.AvatarURL)
		}
		if !slices.Contains(e.media.deleted, first) || mediaStatus(t, e.pool, first) != "gone" {
			t.Errorf("expected the first avatar deleted, deleted=%v status=%s", e.media.deleted, mediaStatus(t, e.pool, first))
		}
	})

	t.Run("sending the current avatar again changes nothing", func(t *testing.T) {
		u := e.otpUser(t)
		avatar := insertMedia(ctx, e.pool, u.id, "pending")
		decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": avatar.String()}), http.StatusOK)

		decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": avatar.String()}), http.StatusOK)

		if mediaStatus(t, e.pool, avatar) != "ready" {
			t.Errorf("expected the avatar kept, got %s", mediaStatus(t, e.pool, avatar))
		}
	})

	t.Run("someone else's or unknown media is a 400", func(t *testing.T) {
		u := e.otpUser(t)
		other := e.otpUser(t)
		foreign := insertMedia(ctx, e.pool, other.id, "pending")

		for _, id := range []uuid.UUID{foreign, uuid.New()} {
			rec := e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": id.String()})
			expectError(t, rec, http.StatusBadRequest, "invalid_avatar")
		}
		if mediaStatus(t, e.pool, foreign) != "pending" {
			t.Error("someone else's media must stay untouched")
		}
	})
}

func TestDeleteAvatar(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)
	avatar := insertMedia(ctx, e.pool, u.id, "pending")
	decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": avatar.String()}), http.StatusOK)

	if rec := e.api.Delete("/profile/avatar", u.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	body := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)
	if body.AvatarURL != nil {
		t.Errorf("expected no avatar, got %v", *body.AvatarURL)
	}
	if mediaStatus(t, e.pool, avatar) != "gone" {
		t.Errorf("expected the avatar media deleted, got %s", mediaStatus(t, e.pool, avatar))
	}

	if rec := e.api.Delete("/profile/avatar", u.bearer); rec.Code != http.StatusNoContent {
		t.Errorf("deleting a missing avatar should still be 204, got %d", rec.Code)
	}
}
