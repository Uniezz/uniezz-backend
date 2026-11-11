package connections

import (
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) match(t *testing.T, a, b testUser) {
	t.Helper()
	e.openTo(t, a, "friends")
	e.openTo(t, b, "friends")
	decode[ActionResult](t, e.act(b, e.discover(t, b, "mode=friends")[0].CandidateID, "friends", "like"), 200)
	result := decode[ActionResult](t, e.act(a, e.discover(t, a, "mode=friends")[0].CandidateID, "friends", "like"), 200)
	if !result.Matched {
		t.Fatalf("no match: %+v", result)
	}
}

type connectionPage struct {
	Items      []Connection
	NextCursor *string
}

func TestListConnections(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.match(t, a, b)

	page := decode[connectionPage](t, e.api.Get("/connections", a.bearer), 200)
	if len(page.Items) != 1 || page.Items[0].Profile.ID != b.id || page.NextCursor != nil {
		t.Fatalf("a's connections = %+v", page)
	}
	page = decode[connectionPage](t, e.api.Get("/connections", b.bearer), 200)
	if len(page.Items) != 1 || page.Items[0].Profile.ID != a.id {
		t.Fatalf("b's connections = %+v", page)
	}

	if rec := e.api.Post("/blocks/"+a.id.String(), b.bearer); rec.Code != 204 {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}
	if page := decode[connectionPage](t, e.api.Get("/connections", a.bearer), 200); len(page.Items) != 0 {
		t.Errorf("blocked connection listed: %+v", page)
	}
}

func TestDeleteConnection(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	outsider := e.user(t)
	e.match(t, a, b)
	id := decode[connectionPage](t, e.api.Get("/connections", a.bearer), 200).Items[0].ID

	expectError(t, e.api.Delete("/connections/"+id.String(), outsider.bearer), 404, "not_found")
	if rec := e.api.Delete("/connections/"+id.String(), b.bearer); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	expectError(t, e.api.Delete("/connections/"+id.String(), a.bearer), 404, "not_found")
	expectError(t, e.api.Delete("/connections/"+uuid.NewString(), a.bearer), 404, "not_found")
}
