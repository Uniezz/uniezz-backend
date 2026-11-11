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
