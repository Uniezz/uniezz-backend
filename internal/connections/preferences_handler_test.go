package connections

import (
	"context"
	"testing"
	"time"
)

func TestSavePreferences(t *testing.T) {
	e := newTestEnv(t)
	u := e.user(t)
	today := time.Now().UTC()
	minor := today.AddDate(-17, 0, 0).Format(time.DateOnly)
	child := today.AddDate(-10, 0, 0).Format(time.DateOnly)
	future := today.AddDate(0, 0, 2).Format(time.DateOnly)

	expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"openTo": []string{"dating"}}), 400, "dating_requires_adult")
	expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": minor, "openTo": []string{"dating"}}), 400, "dating_requires_adult")
	expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": child}), 400, "invalid_input")
	expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": future}), 400, "invalid_input")

	if rec := e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": minor, "openTo": []string{"friends", "friends"}, "intent": " <b>Climbing</b> "}); rec.Code != 204 {
		t.Fatalf("minor friends: %d %s", rec.Code, rec.Body.String())
	}
	expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"openTo": []string{"friends", "dating"}}), 400, "dating_requires_adult")

	var openTo []string
	var intent *string
	err := e.pool.QueryRow(context.Background(), "SELECT open_to, intent FROM connection_profiles WHERE user_id = $1", u.id).Scan(&openTo, &intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(openTo) != 1 || openTo[0] != "friends" || intent == nil || *intent != "Climbing" {
		t.Errorf("saved open_to = %v intent = %v", openTo, intent)
	}

	if rec := e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": "2000-01-01", "openTo": []string{"dating"}, "intent": ""}); rec.Code != 204 {
		t.Fatalf("adult dating: %d %s", rec.Code, rec.Body.String())
	}
	if err := e.pool.QueryRow(context.Background(), "SELECT intent FROM connection_profiles WHERE user_id = $1", u.id).Scan(&intent); err != nil {
		t.Fatal(err)
	}
	if intent != nil {
		t.Errorf("intent = %q, want cleared", *intent)
	}
}

func TestAgeOn(t *testing.T) {
	day := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		birth string
		want  int
	}{
		{"2008-03-01", 18},
		{"2008-03-02", 17},
		{"2008-02-29", 18},
		{"2000-02-29", 26},
	}
	for _, c := range cases {
		birth, _ := time.Parse(time.DateOnly, c.birth)
		if got := ageOn(birth, day); got != c.want {
			t.Errorf("ageOn(%s) = %d, want %d", c.birth, got, c.want)
		}
	}
}
