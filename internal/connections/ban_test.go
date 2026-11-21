package connections

import (
	"context"
	"net/http"
	"testing"
)

func TestBannedStudentsAreHidden(t *testing.T) {
	e := newTestEnv(t)
	ban := func(t *testing.T, u testUser) {
		t.Helper()
		if _, err := e.pool.Exec(context.Background(), "UPDATE users SET banned_at = NOW() WHERE id = $1", u.id); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("a banned connection is not listed", func(t *testing.T) {
		a, b := e.user(t), e.user(t)
		e.match(t, a, b)
		ban(t, b)

		if page := decode[connectionPage](t, e.api.Get("/connections", a.bearer), http.StatusOK); len(page.Items) != 0 {
			t.Errorf("banned connection listed: %+v", page)
		}
	})

	t.Run("a banned student is not a discovery card", func(t *testing.T) {
		a, b := e.user(t), e.user(t)
		e.openTo(t, a, ModeFriends)
		e.openTo(t, b, ModeFriends)
		ban(t, b)

		if cards := e.discover(t, a, "mode=friends"); len(cards) != 0 {
			t.Errorf("cards = %+v, want none", cards)
		}
	})
}
