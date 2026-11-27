package profile

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/google/uuid"
)

func TestProfileFollows(t *testing.T) {
	e := newTestEnv(t)
	follower := e.otpUser(t)
	followed := e.otpUser(t)
	path := "/profiles/" + followed.id.String() + "/follow"

	for range 2 {
		if rec := e.api.Put(path, follower.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("follow: expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	}

	ownProfile := decode[Profile](t, e.api.Get("/profile/me", follower.bearer), http.StatusOK)
	if ownProfile.FollowersCount != 0 || ownProfile.FollowingCount != 1 || ownProfile.FollowedByMe {
		t.Errorf("follower profile counts = (%d, %d), followedByMe=%t", ownProfile.FollowersCount, ownProfile.FollowingCount, ownProfile.FollowedByMe)
	}
	publicProfile := decode[PublicProfile](t, e.api.Get("/profiles/"+followed.id.String(), follower.bearer), http.StatusOK)
	if publicProfile.FollowersCount != 1 || publicProfile.FollowingCount != 0 || !publicProfile.FollowedByMe {
		t.Errorf("followed profile counts = (%d, %d), followedByMe=%t", publicProfile.FollowersCount, publicProfile.FollowingCount, publicProfile.FollowedByMe)
	}
	publicProfile = decode[PublicProfile](t, e.api.Get("/profiles/"+followed.id.String(), followed.bearer), http.StatusOK)
	if publicProfile.FollowedByMe {
		t.Error("followedByMe must be false for a viewer who does not follow the profile")
	}

	type page struct {
		Items      []ProfileSummary `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	following := decode[page](t, e.api.Get("/profile/me/following", follower.bearer), http.StatusOK)
	if len(following.Items) != 1 || following.Items[0].ID != followed.id || following.NextCursor != nil {
		t.Errorf("my following = %+v, want the followed profile only", following)
	}
	followers := decode[page](t, e.api.Get("/profiles/"+followed.id.String()+"/followers", follower.bearer), http.StatusOK)
	if len(followers.Items) != 1 || followers.Items[0].ID != follower.id || followers.NextCursor != nil {
		t.Errorf("profile followers = %+v, want the caller only", followers)
	}

	for range 2 {
		if rec := e.api.Delete(path, follower.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("unfollow: expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
	}
	publicProfile = decode[PublicProfile](t, e.api.Get("/profiles/"+followed.id.String(), follower.bearer), http.StatusOK)
	if publicProfile.FollowersCount != 0 || publicProfile.FollowedByMe {
		t.Errorf("after unfollow: followersCount=%d, followedByMe=%t", publicProfile.FollowersCount, publicProfile.FollowedByMe)
	}
}

func TestFollowListsPaginateNewestFirst(t *testing.T) {
	e := newTestEnv(t)
	target := e.otpUser(t)
	ctx := context.Background()

	var followers, following []uuid.UUID
	for i := range 3 {
		follower := e.otpUser(t)
		followed := e.otpUser(t)
		followers = append(followers, follower.id)
		following = append(following, followed.id)
		_, err := e.pool.Exec(ctx, `
			INSERT INTO profile_follows (follower_id, followed_id, created_at)
			VALUES ($1, $2, NOW() - make_interval(mins => $3)),
			       ($2, $4, NOW() - make_interval(mins => $3))`,
			follower.id, target.id, i+1, followed.id)
		if err != nil {
			t.Fatal(err)
		}
	}
	type page struct {
		Items      []ProfileSummary `json:"items"`
		NextCursor *string          `json:"nextCursor"`
	}
	assertPaged := func(path string, ids []uuid.UUID) {
		t.Helper()
		params := url.Values{"limit": {"2"}}
		var got []uuid.UUID
		for range 3 {
			p := decode[page](t, e.api.Get(path+"?"+params.Encode(), target.bearer), http.StatusOK)
			for _, item := range p.Items {
				got = append(got, item.ID)
			}
			if p.NextCursor == nil {
				break
			}
			params.Set("cursor", *p.NextCursor)
		}
		if len(got) != len(ids) {
			t.Fatalf("%s returned %d profiles, want %d", path, len(got), len(ids))
		}
		for i, id := range ids {
			if got[i] != id {
				t.Errorf("%s item %d = %s, want %s", path, i, got[i], id)
			}
		}
	}

	// Requests are newest-first; the first inserted pair has the newest timestamp.
	wantFollowers := []uuid.UUID{followers[0], followers[1], followers[2]}
	wantFollowing := []uuid.UUID{following[0], following[1], following[2]}
	assertPaged("/profiles/"+target.id.String()+"/followers", wantFollowers)
	assertPaged("/profile/me/followers", wantFollowers)
	assertPaged("/profiles/"+target.id.String()+"/following", wantFollowing)
	assertPaged("/profile/me/following", wantFollowing)
}

func TestFollowRejectsSelfAndUnknownProfiles(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.otpUser(t)
	expectError(t, e.api.Put("/profiles/"+viewer.id.String()+"/follow", viewer.bearer), http.StatusBadRequest, "invalid_input")
	expectError(t, e.api.Put("/profiles/"+uuid.NewString()+"/follow", viewer.bearer), http.StatusNotFound, "not_found")
	expectError(t, e.api.Get("/profiles/"+uuid.NewString()+"/followers", viewer.bearer), http.StatusNotFound, "not_found")
	expectError(t, e.api.Get("/profiles/"+uuid.NewString()+"/following", viewer.bearer), http.StatusNotFound, "not_found")
}

func TestFollowOperationsRequireSession(t *testing.T) {
	e := newTestEnv(t)
	id := uuid.NewString()

	tests := []struct {
		name string
		call func() *httptest.ResponseRecorder
	}{
		{"follow", func() *httptest.ResponseRecorder { return e.api.Put("/profiles/" + id + "/follow") }},
		{"unfollow", func() *httptest.ResponseRecorder { return e.api.Delete("/profiles/" + id + "/follow") }},
		{"list my followers", func() *httptest.ResponseRecorder { return e.api.Get("/profile/me/followers") }},
		{"list my following", func() *httptest.ResponseRecorder { return e.api.Get("/profile/me/following") }},
		{"list profile followers", func() *httptest.ResponseRecorder { return e.api.Get("/profiles/" + id + "/followers") }},
		{"list profile following", func() *httptest.ResponseRecorder { return e.api.Get("/profiles/" + id + "/following") }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if rec := tc.call(); rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func followCounts(t *testing.T, e *testEnv, userID uuid.UUID) (followers, following int) {
	t.Helper()
	err := e.pool.QueryRow(context.Background(), `
		SELECT COALESCE(MAX(followers_count), 0), COALESCE(MAX(following_count), 0)
		FROM profile_follow_counts WHERE user_id = $1`, userID).Scan(&followers, &following)
	if err != nil {
		t.Fatal(err)
	}
	return followers, following
}

func followRows(t *testing.T, e *testEnv, followerID, followedID uuid.UUID) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM profile_follows WHERE follower_id = $1 AND followed_id = $2",
		followerID, followedID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestFollowCounts(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("following at once from many taps stores and counts one follow", func(t *testing.T) {
		follower := e.otpUser(t)
		followed := e.otpUser(t)
		path := "/profiles/" + followed.id.String() + "/follow"

		var wg sync.WaitGroup
		recs := make([]*httptest.ResponseRecorder, 5)
		for i := range recs {
			wg.Go(func() { recs[i] = e.api.Put(path, follower.bearer) })
		}
		wg.Wait()

		for _, rec := range recs {
			if rec.Code != http.StatusNoContent {
				t.Errorf("follow: expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		if n := followRows(t, e, follower.id, followed.id); n != 1 {
			t.Errorf("got %d follow rows, want 1", n)
		}
		if followers, _ := followCounts(t, e, followed.id); followers != 1 {
			t.Errorf("followers_count = %d, want 1", followers)
		}
		if _, following := followCounts(t, e, follower.id); following != 1 {
			t.Errorf("following_count = %d, want 1", following)
		}
	})

	t.Run("unfollowing at once from many taps counts one unfollow", func(t *testing.T) {
		follower := e.otpUser(t)
		followed := e.otpUser(t)
		path := "/profiles/" + followed.id.String() + "/follow"
		if rec := e.api.Put(path, follower.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("follow: %d %s", rec.Code, rec.Body.String())
		}

		var wg sync.WaitGroup
		recs := make([]*httptest.ResponseRecorder, 5)
		for i := range recs {
			wg.Go(func() { recs[i] = e.api.Delete(path, follower.bearer) })
		}
		wg.Wait()

		for _, rec := range recs {
			if rec.Code != http.StatusNoContent {
				t.Errorf("unfollow: expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		if followers, _ := followCounts(t, e, followed.id); followers != 0 {
			t.Errorf("followers_count = %d, want 0", followers)
		}
		if _, following := followCounts(t, e, follower.id); following != 0 {
			t.Errorf("following_count = %d, want 0", following)
		}
	})

	t.Run("unfollowing a student who is deleting their account succeeds", func(t *testing.T) {
		follower := e.otpUser(t)
		followed := e.otpUser(t)
		path := "/profiles/" + followed.id.String() + "/follow"
		if rec := e.api.Put(path, follower.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("follow: %d %s", rec.Code, rec.Body.String())
		}
		if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = NOW(), email = NULL WHERE id = $1", followed.id); err != nil {
			t.Fatal(err)
		}

		if rec := e.api.Delete(path, follower.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("unfollow: expected 204, got %d: %s", rec.Code, rec.Body.String())
		}
		if n := followRows(t, e, follower.id, followed.id); n != 0 {
			t.Errorf("got %d follow rows, want 0", n)
		}
		if _, following := followCounts(t, e, follower.id); following != 0 {
			t.Errorf("following_count = %d, want 0", following)
		}
	})

	t.Run("counts match the lists and leave out students hidden from the viewer", func(t *testing.T) {
		viewer := e.otpUser(t)
		target := e.otpUser(t)
		visible := e.otpUser(t)
		blocked := e.otpUser(t)
		banned := e.otpUser(t)
		for _, u := range []testUser{visible, blocked, banned} {
			if rec := e.api.Put("/profiles/"+target.id.String()+"/follow", u.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("follow: %d %s", rec.Code, rec.Body.String())
			}
			if rec := e.api.Put("/profiles/"+u.id.String()+"/follow", target.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("follow back: %d %s", rec.Code, rec.Body.String())
			}
		}
		if _, err := e.pool.Exec(ctx, "INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2)", viewer.id, blocked.id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = e.pool.Exec(context.Background(), "DELETE FROM blocks WHERE blocker_id = $1", viewer.id)
		})
		if _, err := e.pool.Exec(ctx, "UPDATE users SET banned_at = NOW() WHERE id = $1", banned.id); err != nil {
			t.Fatal(err)
		}

		type page struct {
			Items []ProfileSummary `json:"items"`
		}
		p := decode[PublicProfile](t, e.api.Get("/profiles/"+target.id.String(), viewer.bearer), http.StatusOK)
		followers := decode[page](t, e.api.Get("/profiles/"+target.id.String()+"/followers", viewer.bearer), http.StatusOK)
		following := decode[page](t, e.api.Get("/profiles/"+target.id.String()+"/following", viewer.bearer), http.StatusOK)
		if p.FollowersCount != 1 || len(followers.Items) != 1 {
			t.Errorf("followersCount = %d with %d listed, want 1 and 1", p.FollowersCount, len(followers.Items))
		}
		if p.FollowingCount != 1 || len(following.Items) != 1 {
			t.Errorf("followingCount = %d with %d listed, want 1 and 1", p.FollowingCount, len(following.Items))
		}

		own := decode[Profile](t, e.api.Get("/profile/me", target.bearer), http.StatusOK)
		if own.FollowersCount != 2 || own.FollowingCount != 2 {
			t.Errorf("own counts = (%d, %d), want (2, 2) without the banned student", own.FollowersCount, own.FollowingCount)
		}
	})
}
