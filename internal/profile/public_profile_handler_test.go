package profile

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
)

func TestUpdateMeDetails(t *testing.T) {
	e := newTestEnv(t)
	u := e.otpUser(t)

	body := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"bio":       "<b>Hi</b>, I study CS",
		"interests": []string{"Go", "go", "climbing"},
		"privacy":   map[string]bool{"showEmail": true},
	}), http.StatusOK)

	if body.Bio == nil || *body.Bio != "Hi, I study CS" {
		t.Errorf("bio = %v, want the text without HTML", body.Bio)
	}
	if len(body.Interests) != 2 || body.Interests[0] != "Go" || body.Interests[1] != "climbing" {
		t.Errorf("interests = %v, want [Go climbing]", body.Interests)
	}
	if !body.Privacy["showEmail"] || !body.Privacy["showFaculty"] || !body.Privacy["showYear"] {
		t.Errorf("privacy = %v, want every setting true", body.Privacy)
	}

	body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
		"bio":     "",
		"privacy": map[string]bool{"showYear": false},
	}), http.StatusOK)
	if body.Bio != nil {
		t.Errorf("an empty bio must clear it, got %q", *body.Bio)
	}
	if len(body.Interests) != 2 {
		t.Errorf("interests must stay when not sent, got %v", body.Interests)
	}
	if !body.Privacy["showEmail"] || body.Privacy["showYear"] {
		t.Errorf("privacy must merge, got %v", body.Privacy)
	}

	if rec := e.api.Patch("/profile/me", u.bearer, map[string]any{"privacy": map[string]bool{"showShoeSize": true}}); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown privacy setting: expected 400, got %d", rec.Code)
	}
	expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"interests": []string{"   "}}), http.StatusBadRequest, "invalid_input")
}

func TestGetProfile(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.otpUser(t)
	student := e.newUser(t, "umcs", "public-"+uuid.NewString()+"@umcs.pl")
	ctx := context.Background()

	_, err := e.pool.Exec(ctx, `
		INSERT INTO student_programmes (user_id, faculty_name, programme_name, year_of_study, is_primary)
		VALUES ($1, 'WMFiI', 'Informatyka', 2, TRUE)`, student.id)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("hides the email by default and shows faculty and year", func(t *testing.T) {
		p := decode[PublicProfile](t, e.api.Get("/profiles/"+student.id.String(), viewer.bearer), http.StatusOK)
		if p.Email != nil {
			t.Errorf("email must be hidden by default, got %q", *p.Email)
		}
		if len(p.Programmes) != 1 || p.Programmes[0].Faculty == nil || p.Programmes[0].YearOfStudy == nil {
			t.Errorf("programmes = %+v, want faculty and year shown", p.Programmes)
		}
	})

	t.Run("follows the student's privacy settings", func(t *testing.T) {
		decode[Profile](t, e.api.Patch("/profile/me", student.bearer, map[string]any{
			"privacy": map[string]bool{"showEmail": true, "showFaculty": false, "showYear": false},
		}), http.StatusOK)

		p := decode[PublicProfile](t, e.api.Get("/profiles/"+student.id.String(), viewer.bearer), http.StatusOK)
		if p.Email == nil {
			t.Error("email must be shown")
		}
		if p.Programmes[0].Faculty != nil || p.Programmes[0].YearOfStudy != nil {
			t.Errorf("programmes = %+v, want faculty and year hidden", p.Programmes)
		}
		if p.Programmes[0].Programme != "Informatyka" {
			t.Errorf("programme name must stay, got %q", p.Programmes[0].Programme)
		}
	})

	t.Run("unknown student is 404, no session is 401", func(t *testing.T) {
		expectError(t, e.api.Get("/profiles/"+uuid.NewString(), viewer.bearer), http.StatusNotFound, "not_found")
		if rec := e.api.Get("/profiles/" + student.id.String()); rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
	})
}

func TestListProfiles(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.otpUser(t)
	ctx := context.Background()
	tag := "list" + uuid.NewString()[:8]

	var ids []uuid.UUID
	for i := range 5 {
		u := e.newUser(t, "umcs", "list-"+uuid.NewString()+"@umcs.pl")
		_, err := e.pool.Exec(ctx, "UPDATE users SET last_name = $2, created_at = NOW() - make_interval(mins => $3) WHERE id = $1", u.id, tag, i)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.id)
	}
	other := e.newUser(t, "kul", "list-"+uuid.NewString()+"@student.kul.pl")
	if _, err := e.pool.Exec(ctx, "UPDATE users SET last_name = $2 WHERE id = $1", other.id, tag); err != nil {
		t.Fatal(err)
	}

	type page struct {
		Items      []ProfileSummary `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	list := func(t *testing.T, params url.Values) page {
		t.Helper()
		return decode[page](t, e.api.Get("/profiles?"+params.Encode(), viewer.bearer), http.StatusOK)
	}

	t.Run("walks every page newest first without gaps or repeats", func(t *testing.T) {
		var got []uuid.UUID
		params := url.Values{"name": {tag}, "university": {"umcs"}, "limit": {"2"}}
		for range 10 {
			p := list(t, params)
			for _, item := range p.Items {
				got = append(got, item.ID)
			}
			if p.NextCursor == nil {
				break
			}
			params.Set("cursor", *p.NextCursor)
		}

		if len(got) != len(ids) {
			t.Fatalf("got %d profiles, want %d", len(got), len(ids))
		}
		for i := range ids {
			if got[i] != ids[i] {
				t.Errorf("position %d: got %v, want %v", i, got[i], ids[i])
			}
		}
	})

	t.Run("filters by university and name, treating LIKE wildcards literally", func(t *testing.T) {
		if p := list(t, url.Values{"name": {tag}}); len(p.Items) != 6 {
			t.Errorf("expected 6 profiles across universities, got %d", len(p.Items))
		}
		if p := list(t, url.Values{"name": {tag}, "university": {"kul"}}); len(p.Items) != 1 || p.Items[0].ID != other.id {
			t.Errorf("expected only the KUL profile, got %+v", p.Items)
		}
		if p := list(t, url.Values{"name": {tag + "%"}}); len(p.Items) != 0 {
			t.Errorf("%% must not act as a wildcard, got %d profiles", len(p.Items))
		}
	})

	t.Run("returns the avatar URL of ready avatars only", func(t *testing.T) {
		avatars := map[uuid.UUID]uuid.UUID{}
		for i, status := range []string{"ready", "deleting"} {
			mediaID := insertMedia(ctx, e.pool, ids[i], status)
			if _, err := e.pool.Exec(ctx, "UPDATE users SET avatar_media_id = $2 WHERE id = $1", ids[i], mediaID); err != nil {
				t.Fatal(err)
			}
			avatars[ids[i]] = mediaID
		}

		p := list(t, url.Values{"name": {tag}, "university": {"umcs"}})
		for _, item := range p.Items {
			switch item.ID {
			case ids[0]:
				if item.AvatarURL == nil || *item.AvatarURL != avatarURL(avatars[ids[0]]) {
					t.Errorf("ready avatar: got %v, want %s", item.AvatarURL, avatarURL(avatars[ids[0]]))
				}
			default:
				if item.AvatarURL != nil {
					t.Errorf("profile %v: expected no avatar URL, got %s", item.ID, *item.AvatarURL)
				}
			}
		}
	})

	t.Run("rejects a malformed cursor", func(t *testing.T) {
		expectError(t, e.api.Get("/profiles?cursor=not-a-cursor", viewer.bearer), http.StatusBadRequest, "invalid_cursor")
	})
}
