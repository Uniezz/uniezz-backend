package connections

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestBlockStudent(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, ModeFriends)
	e.openTo(t, b, ModeFriends)

	expectError(t, e.api.Put("/connections/blocks/"+uuid.NewString(), a.bearer), http.StatusNotFound, "not_found")
	expectError(t, e.api.Put(e.blockPath(a), a.bearer), http.StatusBadRequest, "invalid_input")

	for range 2 {
		if rec := e.api.Put(e.blockPath(b), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
		}
	}
	if cards := e.discover(t, b, "mode=friends"); len(cards) != 0 {
		t.Errorf("blocker shown to the blocked student: %+v", cards)
	}
	expectError(t, e.api.Get("/profiles/"+a.id.String(), b.bearer), http.StatusNotFound, "not_found")

	for range 2 {
		if rec := e.api.Delete(e.blockPath(b), a.bearer); rec.Code != http.StatusNoContent {
			t.Fatalf("unblock: %d %s", rec.Code, rec.Body.String())
		}
	}
	if cards := e.discover(t, b, "mode=friends"); len(cards) != 1 {
		t.Errorf("unblocked student not shown: %+v", cards)
	}
}

func TestBlockRemovesFollowsAndPreventsNewOnes(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)

	followPath := "/profiles/" + b.id.String() + "/follow"
	if rec := e.api.Put(followPath, a.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("follow before block: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.api.Put("/profiles/"+a.id.String()+"/follow", b.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("reverse follow before block: %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.api.Put(e.blockPath(b), a.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}

	var count int
	if err := e.pool.QueryRow(t.Context(),
		"SELECT COUNT(*) FROM profile_follows WHERE (follower_id = $1 AND followed_id = $2) OR (follower_id = $2 AND followed_id = $1)",
		a.id, b.id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("blocked pair has %d follows, want none", count)
	}

	for _, caller := range []testUser{a, b} {
		targetID := b.id
		if caller.id == b.id {
			targetID = a.id
		}
		rec := e.api.Put("/profiles/"+targetID.String()+"/follow", caller.bearer)
		if rec.Code != http.StatusNotFound {
			t.Errorf("follow blocked profile: expected 404, got %d: %s", rec.Code, rec.Body.String())
		}
	}
}
