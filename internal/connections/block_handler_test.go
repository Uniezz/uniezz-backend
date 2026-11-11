package connections

import (
	"testing"

	"github.com/google/uuid"
)

func TestBlockStudent(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.openTo(t, a, "friends")
	e.openTo(t, b, "friends")

	expectError(t, e.api.Post("/blocks/"+uuid.NewString(), a.bearer), 404, "not_found")
	expectError(t, e.api.Post("/blocks/"+a.id.String(), a.bearer), 400, "invalid_input")

	for range 2 {
		if rec := e.api.Post("/blocks/"+b.id.String(), a.bearer); rec.Code != 204 {
			t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
		}
	}
	if cards := e.discover(t, b, "mode=friends"); len(cards) != 0 {
		t.Errorf("blocker shown to the blocked student: %+v", cards)
	}
	expectError(t, e.api.Get("/profiles/"+a.id.String(), b.bearer), 404, "not_found")

	for range 2 {
		if rec := e.api.Delete("/blocks/"+b.id.String(), a.bearer); rec.Code != 204 {
			t.Fatalf("unblock: %d %s", rec.Code, rec.Body.String())
		}
	}
	if cards := e.discover(t, b, "mode=friends"); len(cards) != 1 {
		t.Errorf("unblocked student not shown: %+v", cards)
	}
}
