package connections

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"

	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/rewards"

	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSavePreferences(t *testing.T) {
	e := newTestEnv(t)
	today := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	minor := today.AddDate(-17, 0, 0).Format(time.DateOnly)
	child := today.AddDate(-10, 0, 0).Format(time.DateOnly)
	future := today.AddDate(0, 0, 2).Format(time.DateOnly)

	t.Run("a new student has empty preferences", func(t *testing.T) {
		u := e.user(t)
		got := decode[Preferences](t, e.api.Get("/connections/me", u.bearer), http.StatusOK)
		if got.BirthDate != nil || got.Intent != nil || got.OpenTo == nil || len(got.OpenTo) != 0 {
			t.Errorf("preferences = %+v", got)
		}
	})

	t.Run("profile patch applies the same dating age rule", func(t *testing.T) {
		u := e.user(t)
		if _, err := e.pool.Exec(context.Background(), `UPDATE users SET gender = NULL WHERE id = $1`, u.id); err != nil {
			t.Fatalf("clear fixture gender: %v", err)
		}

		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{
			"birthDate": minor,
			"gender":    "male",
			"openTo":    []string{"dating"},
		}), http.StatusBadRequest, "dating_requires_adult")

		var gender *string
		if err := e.pool.QueryRow(context.Background(), `SELECT gender FROM users WHERE id = $1`, u.id).Scan(&gender); err != nil {
			t.Fatalf("read fixture gender: %v", err)
		}
		if gender != nil {
			t.Errorf("gender = %v after rejected patch, want null", *gender)
		}
	})

	t.Run("profile intent does not opt the student into discovery", func(t *testing.T) {
		u := e.user(t)
		decode[map[string]any](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"intent": "Chess"}), http.StatusOK)

		got := decode[Preferences](t, e.api.Get("/connections/me", u.bearer), http.StatusOK)
		if len(got.OpenTo) != 0 {
			t.Errorf("openTo = %v after setting intent, want empty", got.OpenTo)
		}
	})

	t.Run("both profile endpoints enforce the shared intent limit", func(t *testing.T) {
		u := e.user(t)
		intent := strings.Repeat("x", maxIntentLength+1)

		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"intent": intent}), http.StatusBadRequest, "invalid_input")
		expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"intent": intent}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("profile patch returns openTo in canonical order without duplicates", func(t *testing.T) {
		u := e.user(t)
		got := decode[struct {
			OpenTo []Mode `json:"openTo"`
		}](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
			"openTo": []string{"studyBuddy", "friends", "friends"},
		}), http.StatusOK)
		if !slices.Equal(got.OpenTo, []Mode{ModeFriends, ModeStudyBuddy}) {
			t.Errorf("openTo = %v, want %v", got.OpenTo, []Mode{ModeFriends, ModeStudyBuddy})
		}
	})

	t.Run("dating needs an adult birth date", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"openTo": []string{"dating"}}), http.StatusBadRequest, "dating_requires_profile")
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": minor, "openTo": []string{"dating"}}), http.StatusBadRequest, "dating_requires_adult")

		decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": minor, "openTo": []string{"friends"}}), http.StatusOK)
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"openTo": []string{"friends", "dating"}}), http.StatusBadRequest, "dating_requires_adult")
	})

	t.Run("a birth date outside 16 to 100 years is a 400", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": child}), http.StatusBadRequest, "invalid_input")
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": future}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("saved values come back cleaned and a missing field keeps its value", func(t *testing.T) {
		u := e.user(t)
		got := decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{
			"birthDate": "2000-01-01", "openTo": []string{"friends", "dating", "friends"}, "intent": " <b>Climbing</b> ",
		}), http.StatusOK)
		if got.BirthDate == nil || *got.BirthDate != "2000-01-01" || len(got.OpenTo) != 2 || got.Intent == nil || *got.Intent != "Climbing" {
			t.Fatalf("saved = %+v", got)
		}
		if !slices.Equal(got.OpenTo, []Mode{ModeDating, ModeFriends}) {
			t.Errorf("openTo = %v, want sorted unique modes", got.OpenTo)
		}

		got = decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"intent": ""}), http.StatusOK)
		if got.Intent != nil || len(got.OpenTo) != 2 || got.BirthDate == nil {
			t.Errorf("after clearing the intent = %+v", got)
		}
		if again := decode[Preferences](t, e.api.Get("/connections/me", u.bearer), http.StatusOK); again.Intent != nil || len(again.OpenTo) != 2 {
			t.Errorf("read back = %+v", again)
		}
	})

	t.Run("a birth date can be set only once and sending it again changes nothing", func(t *testing.T) {
		u := e.user(t)
		decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": "2000-01-01"}), http.StatusOK)

		again := decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": "2000-01-01", "intent": "Chess"}), http.StatusOK)
		if again.BirthDate == nil || *again.BirthDate != "2000-01-01" || again.Intent == nil || *again.Intent != "Chess" {
			t.Errorf("same date again = %+v", again)
		}

		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": "2001-02-03", "intent": "Go"}), http.StatusBadRequest, "birth_date_immutable")
		got := decode[Preferences](t, e.api.Get("/connections/me", u.bearer), http.StatusOK)
		if got.BirthDate == nil || *got.BirthDate != "2000-01-01" || got.Intent == nil || *got.Intent != "Chess" {
			t.Errorf("after a rejected change = %+v", got)
		}
	})

	t.Run("two different first birth dates at once save exactly one", func(t *testing.T) {
		u := e.user(t)
		dates := []string{"2000-01-01", "2001-02-03"}
		codes := make([]int, len(dates))
		var wg sync.WaitGroup
		for i, date := range dates {
			wg.Go(func() { codes[i] = e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": date}).Code })
		}
		wg.Wait()

		var saved time.Time
		if err := e.pool.QueryRow(context.Background(), "SELECT birth_date FROM connection_profiles WHERE user_id = $1", u.id).Scan(&saved); err != nil {
			t.Fatalf("read birth date: %v", err)
		}
		winner := -1
		for i, code := range codes {
			if code == http.StatusOK {
				winner = i
			} else if code != http.StatusBadRequest {
				t.Errorf("%s: status %d", dates[i], code)
			}
		}
		if winner < 0 || codes[0] == codes[1] || saved.Format(time.DateOnly) != dates[winner] {
			t.Errorf("codes = %v, saved = %s", codes, saved.Format(time.DateOnly))
		}
	})
}

func TestOperationsNeedASession(t *testing.T) {
	e := newTestEnv(t)
	id := uuid.NewString()
	requests := []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"get preferences", e.api.Get("/connections/me")},
		{"save preferences", e.api.Patch("/connections/me", map[string]any{"openTo": []string{"friends"}})},
		{"discover", e.api.Get("/connections/discover?mode=friends")},
		{"act on a candidate", e.api.Put("/connections/candidates/"+id, map[string]any{"mode": "friends", "action": "like"})},
		{"list connections", e.api.Get("/connections")},
		{"delete a connection", e.api.Delete("/connections/" + id)},
		{"block", e.api.Put("/connections/blocks/" + id)},
		{"unblock", e.api.Delete("/connections/blocks/" + id)},
	}
	for _, tc := range requests {
		t.Run(tc.name, func(t *testing.T) {
			expectError(t, tc.rec, http.StatusUnauthorized, "unauthorized")
		})
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

func TestBirthDateCoins(t *testing.T) {
	e := newTestEnv(t)
	coins := func(t *testing.T, u testUser) int {
		t.Helper()
		m, err := rewards.New(e.pool, jobs.NewRunner(e.pool, 1, 0))
		if err != nil {
			t.Fatalf("rewards.New: %v", err)
		}
		n, err := m.Balance(context.Background(), u.id)
		if err != nil {
			t.Fatalf("balance: %v", err)
		}
		return n
	}

	t.Run("the first birth date pays 10 coins once", func(t *testing.T) {
		u := e.user(t)
		type saved struct {
			CoinsEarned int `json:"coinsEarned"`
		}
		first := decode[saved](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": "2000-01-01"}), http.StatusOK)
		again := decode[saved](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": "2000-01-01"}), http.StatusOK)
		if first.CoinsEarned != 10 || again.CoinsEarned != 0 {
			t.Errorf("coinsEarned first %d, again %d; want 10, 0", first.CoinsEarned, again.CoinsEarned)
		}
		if got := coins(t, u); got != 10 {
			t.Errorf("coins = %d, want 10", got)
		}
	})

	t.Run("setting birth date through profile pays once and accepts the same date", func(t *testing.T) {
		u := e.user(t)
		type profileUpdate struct {
			BirthDate   *string `json:"birthDate"`
			Intent      *string `json:"intent"`
			CoinsEarned int     `json:"coinsEarned"`
		}

		first := decode[profileUpdate](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"birthDate": "2000-01-01"}), http.StatusOK)
		again := decode[profileUpdate](t, e.api.Patch("/profile/me", u.bearer, map[string]any{
			"birthDate": "2000-01-01",
			"intent":    "Chess",
		}), http.StatusOK)
		if first.CoinsEarned != 10 || again.CoinsEarned != 0 {
			t.Errorf("coinsEarned first %d, again %d; want 10, 0", first.CoinsEarned, again.CoinsEarned)
		}
		if again.BirthDate == nil || *again.BirthDate != "2000-01-01" || again.Intent == nil || *again.Intent != "Chess" {
			t.Errorf("same-date profile patch = %+v", again)
		}
		if got := coins(t, u); got != 10 {
			t.Errorf("coins = %d, want 10", got)
		}
	})

	t.Run("saving other preferences pays nothing", func(t *testing.T) {
		u := e.user(t)
		decode[Preferences](t, e.api.Patch("/connections/me", u.bearer, map[string]any{"intent": "Chess", "openTo": []string{"friends"}}), http.StatusOK)
		if got := coins(t, u); got != 0 {
			t.Errorf("coins = %d, want 0", got)
		}
	})

	t.Run("a rejected birth date pays nothing", func(t *testing.T) {
		u := e.user(t)
		minor := "2009-03-01"
		expectError(t, e.api.Patch("/connections/me", u.bearer, map[string]any{"birthDate": minor, "openTo": []string{"dating"}}), http.StatusBadRequest, "dating_requires_adult")
		if got := coins(t, u); got != 0 {
			t.Errorf("coins = %d, want 0", got)
		}
	})
}
