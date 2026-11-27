package profile

import (
	"context"
	"net/http"
	"testing"
)

func TestBlockedText(t *testing.T) {
	e := newTestEnv(t)

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{name: "a blocked first name", body: map[string]any{"firstName": "Kurwa"}},
		{name: "a spaced-out last name", body: map[string]any{"lastName": "c h u j"}},
		{name: "a leetspeak bio", body: map[string]any{"bio": "Lubię sh1t i matematykę"}},
		{name: "a blocked interest", body: map[string]any{"interests": []string{"climbing", "wh0re"}}},
	} {
		t.Run(tc.name+" is rejected and nothing is saved", func(t *testing.T) {
			u := e.otpUser(t)

			expectError(t, e.api.Patch("/profile/me", u.bearer, tc.body), http.StatusBadRequest, "content_not_allowed")

			var firstName, lastName string
			var bioIsNull bool
			var interests int
			err := e.pool.QueryRow(context.Background(),
				"SELECT first_name, last_name, bio IS NULL, COALESCE(cardinality(interests), 0) FROM users WHERE id = $1", u.id,
			).Scan(&firstName, &lastName, &bioIsNull, &interests)
			if err != nil {
				t.Fatalf("read user: %v", err)
			}
			if firstName != "Jan" || lastName != "Kowalski" || !bioIsNull || interests != 0 {
				t.Errorf("profile changed: %q %q, bio null %v, %d interests", firstName, lastName, bioIsNull, interests)
			}
		})
	}

	t.Run("a real surname that contains a blocked root can be sent again", func(t *testing.T) {
		u := e.otpUser(t)
		if _, err := e.pool.Exec(context.Background(), "UPDATE users SET last_name = 'Murzyński', interests = '{kurwa}' WHERE id = $1", u.id); err != nil {
			t.Fatalf("set surname: %v", err)
		}

		body := map[string]any{"firstName": "Jan", "lastName": "Murzyński", "bio": "Lubię matematykę", "interests": []string{"kurwa", "szachy"}}
		got := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, body), http.StatusOK)
		if got.LastName == nil || *got.LastName != "Murzyński" || got.Bio == nil || len(got.Interests) != 2 {
			t.Errorf("profile = %+v", got)
		}
	})

	t.Run("a blocked surname that is not the current one is rejected", func(t *testing.T) {
		u := e.otpUser(t)
		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"lastName": "Murzyński"}), http.StatusBadRequest, "content_not_allowed")
	})

	t.Run("a blocked programme or faculty is rejected", func(t *testing.T) {
		u := e.otpUser(t)
		for _, body := range []map[string]any{
			{"faculty": "Wydział", "programme": "kurwa"},
			{"faculty": "Wydział c.h.u.j", "programme": "Informatyka"},
		} {
			expectError(t, e.api.Post("/profile/programmes", u.bearer, body), http.StatusBadRequest, "content_not_allowed")
		}
		var n int
		if err := e.pool.QueryRow(context.Background(), "SELECT count(*) FROM student_programmes WHERE user_id = $1", u.id).Scan(&n); err != nil {
			t.Fatalf("count programmes: %v", err)
		}
		if n != 0 {
			t.Errorf("stored %d programmes, want 0", n)
		}
	})

	t.Run("an ordinary profile in several languages is accepted", func(t *testing.T) {
		u := e.otpUser(t)
		body := map[string]any{
			"firstName": "Ołeksandr",
			"bio":       "Студентка першого курсу, люблю шахи. Mi computadora es vieja.",
			"interests": []string{"szachy", "Schlüssel", "climbing"},
		}
		got := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, body), http.StatusOK)
		if got.FirstName == nil || *got.FirstName != "Ołeksandr" || got.Bio == nil || len(got.Interests) != 3 {
			t.Errorf("profile = %+v", got)
		}
	})
}
