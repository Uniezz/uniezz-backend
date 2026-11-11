package connections

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

type connectionPage struct {
	Items      []Connection
	NextCursor *string
}

func TestListConnections(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	e.match(t, a, b)

	page := decode[connectionPage](t, e.api.Get("/connections", a.bearer), http.StatusOK)
	if len(page.Items) != 1 || page.Items[0].Profile.ID != b.id || page.NextCursor != nil {
		t.Fatalf("a's connections = %+v", page)
	}
	page = decode[connectionPage](t, e.api.Get("/connections", b.bearer), http.StatusOK)
	if len(page.Items) != 1 || page.Items[0].Profile.ID != a.id {
		t.Fatalf("b's connections = %+v", page)
	}

	if rec := e.api.Put(e.blockPath(a), b.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("block: %d %s", rec.Code, rec.Body.String())
	}
	if page := decode[connectionPage](t, e.api.Get("/connections", a.bearer), http.StatusOK); len(page.Items) != 0 {
		t.Errorf("blocked connection listed: %+v", page)
	}
}

func TestDeleteConnection(t *testing.T) {
	e := newTestEnv(t)
	a := e.user(t)
	b := e.user(t)
	outsider := e.user(t)
	e.match(t, a, b)
	id := decode[connectionPage](t, e.api.Get("/connections", a.bearer), http.StatusOK).Items[0].ID

	expectError(t, e.api.Delete("/connections/"+id.String(), outsider.bearer), http.StatusNotFound, "not_found")
	if rec := e.api.Delete("/connections/"+id.String(), b.bearer); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	expectError(t, e.api.Delete("/connections/"+id.String(), a.bearer), http.StatusNotFound, "not_found")
	expectError(t, e.api.Delete("/connections/"+uuid.NewString(), a.bearer), http.StatusNotFound, "not_found")
}
