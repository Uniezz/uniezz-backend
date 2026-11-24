package profile

import (
	"context"
	"net/http"
	"net/url"
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
