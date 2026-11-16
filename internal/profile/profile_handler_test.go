package profile

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
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

		want := []string{"avatarUrl", "bannerUrl", "bio", "birthDate", "coins", "createdAt", "email", "firstName", "gender", "id", "intent", "interests", "language", "lastName", "openTo", "privacy", "programmes", "songUrl", "universityId"}

		if !slices.Equal(keys, want) {
			t.Errorf("expected keys %v, got %v", want, keys)
		}
		if body["id"] != u.id.String() || body["universityId"] != "kul" || body["firstName"] != "Jan" {
			t.Errorf("unexpected profile %v", body)
		}
		if programmes, ok := body["programmes"].([]any); !ok || len(programmes) != 0 {
			t.Errorf("expected programmes [], got %v", body["programmes"])
		}
		if body["gender"] != nil {
			t.Errorf("expected gender null, got %v", body["gender"])
		}
		if body["avatarUrl"] != nil {
			t.Errorf("expected avatarUrl null, got %v", body["avatarUrl"])
		}
		if body["birthDate"] != nil {
			t.Errorf("expected birthDate null, got %v", body["birthDate"])
		}
		if body["intent"] != nil {
			t.Errorf("expected intent null, got %v", body["intent"])
		}

		openTo, ok := body["openTo"].([]any)
		if !ok || len(openTo) != 2 {
			t.Fatalf("expected openTo with two modes, got %v", body["openTo"])
		}
		modes := make(map[string]bool, len(openTo))
		for _, value := range openTo {
			mode, ok := value.(string)
			if !ok {
				t.Fatalf("expected openTo values to be strings, got %v", value)
			}
			modes[mode] = true
		}
		if !modes["friends"] || !modes["study_buddy"] {
			t.Errorf("expected friends and study_buddy, got %v", openTo)
		}
	})

	t.Run("lists every programme, primary first, and the avatar URL", func(t *testing.T) {
		u := e.newUser(t, "umcs", "profile-"+uuid.NewString()+"@umcs.pl")
		ctx := context.Background()

		err := database.New(e.pool).WithTx(ctx, func(ctx context.Context) error {
			_, err := e.module.InitProfile(ctx, u.id, auth.InitialProfile{Programmes: []auth.InitialProgramme{
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
		if body.AvatarURL == nil || *body.AvatarURL != avatarURL(avatarID) {
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
		if body.AvatarURL == nil || *body.AvatarURL != avatarURL(first) {
			t.Fatalf("expected the first avatar, got %v", body.AvatarURL)
		}

		body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"avatarMediaId": second.String()}), http.StatusOK)
		if body.AvatarURL == nil || *body.AvatarURL != avatarURL(second) {
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

	t.Run("sets a banner next to the avatar, then replaces it and deletes the old one", func(t *testing.T) {
		u := e.otpUser(t)
		avatar := insertMedia(ctx, e.pool, u.id, "pending")
		first := insertMediaOf(ctx, e.pool, u.id, media.KindBanner, "pending")
		second := insertMediaOf(ctx, e.pool, u.id, media.KindBanner, "pending")

		body := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
			"avatarMediaId": avatar.String(), "bannerMediaId": first.String(),
		}), http.StatusOK)
		if body.BannerURL == nil || *body.BannerURL != bannerURL(first) || body.AvatarURL == nil || *body.AvatarURL != avatarURL(avatar) {
			t.Fatalf("expected the avatar and the first banner, got %v %v", body.AvatarURL, body.BannerURL)
		}

		body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"bannerMediaId": second.String()}), http.StatusOK)
		if body.BannerURL == nil || *body.BannerURL != bannerURL(second) {
			t.Fatalf("expected the second banner, got %v", body.BannerURL)
		}
		if mediaStatus(t, e.pool, first) != "gone" || mediaStatus(t, e.pool, avatar) != "ready" {
			t.Errorf("expected only the first banner deleted, banner=%s avatar=%s", mediaStatus(t, e.pool, first), mediaStatus(t, e.pool, avatar))
		}
	})

	t.Run("an avatar upload is not a banner", func(t *testing.T) {
		u := e.otpUser(t)
		avatar := insertMedia(ctx, e.pool, u.id, "pending")

		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"bannerMediaId": avatar.String()}), http.StatusBadRequest, "invalid_banner")
		if mediaStatus(t, e.pool, avatar) != "pending" {
			t.Error("the avatar upload must stay untouched")
		}
	})
}

func TestUpdateConnectionProfileFields(t *testing.T) {
	e := newTestEnv(t)
	u := e.otpUser(t)

	got := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"birthDate": "2000-01-15",
		"openTo":    []string{"friends", "study_buddy"},
		"intent":    "Looking for a climbing partner",
	}), http.StatusOK)

	if got.BirthDate == nil || *got.BirthDate != "2000-01-15" {
		t.Errorf("birthDate = %v, want 2000-01-15", got.BirthDate)
	}
	if !slices.Equal(got.OpenTo, []string{"friends", "study_buddy"}) {
		t.Errorf("openTo = %v, want friends and study_buddy", got.OpenTo)
	}
	if got.Intent == nil || *got.Intent != "Looking for a climbing partner" {
		t.Errorf("intent = %v, want the submitted text", got.Intent)
	}

	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"birthDate": "2001-01-15",
	}), http.StatusBadRequest, "birth_date_immutable")
	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"birthDate": nil,
	}), http.StatusBadRequest, "birth_date_immutable")
	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"openTo": []string{"dating"},
	}), http.StatusBadRequest, "dating_requires_profile")

	uWithoutDate := e.otpUser(t)
	decode[Profile](t, e.api.Patch("/profile/me", uWithoutDate.bearer, map[string]any{
		"gender": "female",
	}), http.StatusOK)
	expectError(t, e.api.Patch("/profile/me", uWithoutDate.bearer, map[string]any{
		"openTo": []string{"dating"},
	}), http.StatusBadRequest, "dating_requires_profile")

	uReady := e.otpUser(t)
	datingProfile := decode[Profile](t, e.api.Patch("/profile/me", uReady.bearer, map[string]any{
		"birthDate": "2000-01-15",
		"gender":    "female",
		"openTo":    []string{"dating"},
	}), http.StatusOK)

	if datingProfile.BirthDate == nil || *datingProfile.BirthDate != "2000-01-15" {
		t.Errorf("birthDate = %v, want 2000-01-15", datingProfile.BirthDate)
	}
	if datingProfile.Gender == nil || *datingProfile.Gender != GenderFemale {
		t.Errorf("gender = %v, want female", datingProfile.Gender)
	}
	if !slices.Equal(datingProfile.OpenTo, []string{"dating"}) {
		t.Errorf("openTo = %v, want dating", datingProfile.OpenTo)
	}
}

func TestUpdateIntentPlainTextAndLength(t *testing.T) {
	e := newTestEnv(t)
	u := e.otpUser(t)

	got := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"intent": " <b>Climbing</b> ",
	}), http.StatusOK)
	if got.Intent == nil || *got.Intent != "Climbing" {
		t.Errorf("intent = %v, want Climbing", got.Intent)
	}

	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"intent": strings.Repeat("x", 151),
	}), http.StatusBadRequest, "invalid_input")
}

func TestAddProfilePhoto(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)
	photo := insertMedia(ctx, e.pool, u.id, "pending")

	rec := e.api.Post("/profile/photos", u.bearer, map[string]any{
		"mediaId": photo.String(),
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if mediaStatus(t, e.pool, photo) != "ready" {
		t.Fatalf("expected uploaded photo to be claimed, got %s", mediaStatus(t, e.pool, photo))
	}

	var position int
	if err := e.pool.QueryRow(ctx,
		`SELECT position FROM profile_photos WHERE user_id = $1 AND media_id = $2`,
		u.id, photo,
	).Scan(&position); err != nil {
		t.Fatalf("photo was not saved: %v", err)
	}
	if position != 1 {
		t.Errorf("position = %d, want 1", position)
	}
}

func TestProfilePhotoLimitIncludesAvatar(t *testing.T) {
	e := newTestEnv(t)

	u := e.otpUser(t)
	avatar := insertMedia(context.Background(), e.pool, u.id, "pending")
	decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"avatarMediaId": avatar.String(),
	}), http.StatusOK)

	for i := 0; i < 5; i++ {
		photo := insertMedia(context.Background(), e.pool, u.id, "pending")
		rec := e.api.Post("/profile/photos", u.bearer, map[string]any{
			"mediaId": photo.String(),
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("photo %d: expected 204, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
	}

	extra := insertMedia(context.Background(), e.pool, u.id, "pending")
	expectError(t, e.api.Post("/profile/photos", u.bearer, map[string]any{
		"mediaId": extra.String(),
	}), http.StatusBadRequest, "profile_photo_limit")
}

func TestDeleteProfilePhotoCompactsPositions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)

	first := insertMedia(ctx, e.pool, u.id, "pending")
	second := insertMedia(ctx, e.pool, u.id, "pending")

	for _, photo := range []uuid.UUID{first, second} {
		rec := e.api.Post("/profile/photos", u.bearer, map[string]any{
			"mediaId": photo.String(),
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("adding photo: expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	rec := e.api.Delete("/profile/photos/"+first.String(), u.bearer)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("deleting photo: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if mediaStatus(t, e.pool, first) != "gone" {
		t.Errorf("expected deleted photo media to be gone")
	}

	var position int
	if err := e.pool.QueryRow(ctx,
		`SELECT position FROM profile_photos WHERE user_id = $1 AND media_id = $2`,
		u.id, second,
	).Scan(&position); err != nil {
		t.Fatalf("remaining photo was not found: %v", err)
	}
	if position != 1 {
		t.Errorf("remaining photo position = %d, want 1", position)
	}
}

func TestReorderProfilePhotos(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)

	first := insertMedia(ctx, e.pool, u.id, "pending")
	second := insertMedia(ctx, e.pool, u.id, "pending")
	third := insertMedia(ctx, e.pool, u.id, "pending")

	for _, photo := range []uuid.UUID{first, second, third} {
		rec := e.api.Post("/profile/photos", u.bearer, map[string]any{
			"mediaId": photo.String(),
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("adding photo: expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	rec := e.api.Put("/profile/photos", u.bearer, map[string]any{
		"mediaIds": []string{third.String(), first.String(), second.String()},
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("reordering photos: expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	for wantPosition, photo := range []uuid.UUID{third, first, second} {
		var gotPosition int
		if err := e.pool.QueryRow(ctx,
			`SELECT position FROM profile_photos WHERE user_id = $1 AND media_id = $2`,
			u.id, photo,
		).Scan(&gotPosition); err != nil {
			t.Fatalf("reading photo position: %v", err)
		}
		if gotPosition != wantPosition+1 {
			t.Errorf("photo %s position = %d, want %d", photo, gotPosition, wantPosition+1)
		}
	}
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

func TestDeleteBanner(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)
	avatar := insertMedia(ctx, e.pool, u.id, "pending")
	banner := insertMediaOf(ctx, e.pool, u.id, media.KindBanner, "pending")
	decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"avatarMediaId": avatar.String(), "bannerMediaId": banner.String(),
	}), http.StatusOK)

	if rec := e.api.Delete("/profile/banner", u.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	body := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)
	if body.BannerURL != nil || body.AvatarURL == nil {
		t.Errorf("expected no banner and the avatar kept, got banner %v avatar %v", body.BannerURL, body.AvatarURL)
	}
	if mediaStatus(t, e.pool, banner) != "gone" {
		t.Errorf("expected the banner media deleted, got %s", mediaStatus(t, e.pool, banner))
	}

	if rec := e.api.Delete("/profile/banner", u.bearer); rec.Code != http.StatusNoContent {
		t.Errorf("deleting a missing banner should still be 204, got %d", rec.Code)
	}
}

func TestProfileLanguage(t *testing.T) {
	e := newTestEnv(t)
	u := e.otpUser(t)
	p := decode[Profile](t, e.api.Get("/profile/me", u.bearer), 200)
	if p.Language != "pl" {
		t.Fatalf("default language = %q", p.Language)
	}
	for _, language := range []Language{LanguageEnglish, LanguagePolish, LanguageUkrainian, LanguageRussian, LanguageBelarusian, LanguageGerman, LanguageSpanish} {
		p = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"language": language}), 200)
		if p.Language != language {
			t.Errorf("updated language = %q", p.Language)
		}
		// Another device has its own session for the same account.
		token := auth.GenerateSessionToken()
		hash := token.Hash()
		if _, err := auth.NewSessionRepository(database.New(e.pool)).CreateSession(context.Background(), u.id, hash[:], time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		p = decode[Profile](t, e.api.Get("/profile/me", "Authorization: Bearer "+token.String()), 200)
		if p.Language != language {
			t.Errorf("saved language = %q", p.Language)
		}
	}
	for _, language := range []string{"", "fr", "EN", "DE", "en-US"} {
		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"language": language}), 400, "invalid_input")
	}
	p = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"firstName": "Anna"}), 200)
	if p.Language != LanguageSpanish {
		t.Error("unrelated patch reset language")
	}
}

func TestAvatarCountsTowardProfilePhotoLimit(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)

	for i := 0; i < 6; i++ {
		photo := insertMedia(ctx, e.pool, u.id, "pending")
		rec := e.api.Post("/profile/photos", u.bearer, map[string]any{
			"mediaId": photo.String(),
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("photo %d: expected 204, got %d: %s", i+1, rec.Code, rec.Body.String())
		}
	}

	avatar := insertMedia(ctx, e.pool, u.id, "pending")
	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"avatarMediaId": avatar.String(),
	}), http.StatusBadRequest, "profile_photo_limit")
}
