package rewards

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/google/uuid"
)

// Challenges are global, and other packages' tests run against the same
// database at the same time. Paying tests therefore use professor_rated, which
// no other test checks coins for, and the rest use a target nobody reaches.
const unreachable = 1000

func (e *testEnv) admin(t *testing.T) testUser {
	t.Helper()
	u := e.user(t)
	if _, err := e.pool.Exec(context.Background(), "INSERT INTO user_roles (user_id, role) VALUES ($1, $2)", u.id, auth.RoleAdmin); err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	return u
}

func (e *testEnv) challenge(t *testing.T, admin testUser, body map[string]any) Challenge {
	t.Helper()
	full := map[string]any{
		"action": string(ActionProfessorRated), "target": 2, "period": "day", "reward": 15,
		"texts": map[string]any{"pl": map[string]any{"title": "Oceń prowadzących"}},
	}
	for k, v := range body {
		full[k] = v
	}
	c := decode[Challenge](t, e.api.Post("/rewards/challenges", admin.bearer, full), http.StatusCreated)
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM challenges WHERE id = $1", c.ID) })
	return c
}

func (e *testEnv) progressOf(t *testing.T, userID, challengeID uuid.UUID) (int, bool) {
	t.Helper()
	var progress int
	var completed bool
	err := e.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(progress), 0)::int, COALESCE(bool_or(completed_at IS NOT NULL), FALSE)
		FROM challenge_progress WHERE user_id = $1 AND challenge_id = $2`, userID, challengeID).Scan(&progress, &completed)
	if err != nil {
		t.Fatal(err)
	}
	return progress, completed
}

func (e *testEnv) challengeCoins(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	var coins int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT COALESCE(SUM(amount), 0)::int FROM coin_grants WHERE user_id = $1 AND reason = $2", userID, ReasonChallenge).Scan(&coins); err != nil {
		t.Fatal(err)
	}
	return coins
}

func TestChallengeAdmin(t *testing.T) {
	e := newTestEnv(t)

	t.Run("an admin creates, reads, edits and archives a challenge", func(t *testing.T) {
		admin := e.admin(t)
		created := e.challenge(t, admin, map[string]any{
			"texts": map[string]any{
				"pl": map[string]any{"title": " <b>Polub</b> 5 postów ", "description": "W tym tygodniu"},
				"en": map[string]any{"title": "Like 5 posts", "description": ""},
			},
		})
		if created.Texts["pl"].Title != "Polub 5 postów" || created.Texts["en"].Description != nil || created.ArchivedAt != nil {
			t.Errorf("created = %+v", created)
		}

		path := "/rewards/challenges/" + created.ID.String()
		if got := decode[Challenge](t, e.api.Get(path, admin.bearer), http.StatusOK); got.ID != created.ID || got.Target != 2 {
			t.Errorf("got = %+v", got)
		}

		updated := decode[Challenge](t, e.api.Patch(path, admin.bearer, map[string]any{"reward": 40}), http.StatusOK)
		if updated.Reward != 40 || updated.Texts["en"].Title != "Like 5 posts" || updated.Target != 2 {
			t.Errorf("updated = %+v", updated)
		}

		for range 2 {
			if rec := e.api.Delete(path, admin.bearer); rec.Code != http.StatusNoContent {
				t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
			}
		}
		if got := decode[Challenge](t, e.api.Get(path, admin.bearer), http.StatusOK); got.ArchivedAt == nil {
			t.Error("challenge is not archived")
		}
	})

	t.Run("invalid challenges are refused", func(t *testing.T) {
		admin := e.admin(t)
		now := time.Now().UTC()
		for name, body := range map[string]map[string]any{
			"no polish text":       {"texts": map[string]any{"en": map[string]any{"title": "Like"}}},
			"a bad language code":  {"texts": map[string]any{"pl": map[string]any{"title": "Polub"}, "english": map[string]any{"title": "Like"}}},
			"a title of only HTML": {"texts": map[string]any{"pl": map[string]any{"title": "<b></b>"}}},
			"a title too long":     {"texts": map[string]any{"pl": map[string]any{"title": strings.Repeat("a", maxChallengeTitle+1)}}},
			"an end before start":  {"startsAt": now.Add(time.Hour), "endsAt": now},
			"an unknown action":    {"action": "chat_sent"},
			"a zero target":        {"target": 0},
		} {
			t.Run(name, func(t *testing.T) {
				full := map[string]any{"action": string(ActionProfessorRated), "target": 2, "period": "day", "reward": 15,
					"texts": map[string]any{"pl": map[string]any{"title": "Oceń"}}}
				for k, v := range body {
					full[k] = v
				}
				expectError(t, e.api.Post("/rewards/challenges", admin.bearer, full), http.StatusBadRequest, "invalid_input")
			})
		}
	})

	t.Run("an edit cannot end a challenge before it starts", func(t *testing.T) {
		admin := e.admin(t)
		now := time.Now().UTC()
		c := e.challenge(t, admin, map[string]any{"startsAt": now})

		expectError(t, e.api.Patch("/rewards/challenges/"+c.ID.String(), admin.bearer, map[string]any{"endsAt": now.Add(-time.Hour)}), http.StatusBadRequest, "invalid_input")
	})

	t.Run("an unknown challenge is not found", func(t *testing.T) {
		admin := e.admin(t)
		path := "/rewards/challenges/" + uuid.NewString()

		expectError(t, e.api.Get(path, admin.bearer), http.StatusNotFound, "not_found")
		expectError(t, e.api.Patch(path, admin.bearer, map[string]any{"reward": 1}), http.StatusNotFound, "not_found")
		expectError(t, e.api.Delete(path, admin.bearer), http.StatusNotFound, "not_found")
	})

	t.Run("only admins manage challenges", func(t *testing.T) {
		student, moderator := e.user(t), e.user(t)
		if _, err := e.pool.Exec(context.Background(), "INSERT INTO user_roles (user_id, role) VALUES ($1, $2)", moderator.id, auth.RoleModerator); err != nil {
			t.Fatal(err)
		}
		id := uuid.NewString()
		for _, caller := range []testUser{student, moderator} {
			expectError(t, e.api.Get("/rewards/challenges", caller.bearer), http.StatusForbidden, "forbidden")
			expectError(t, e.api.Post("/rewards/challenges", caller.bearer, map[string]any{}), http.StatusForbidden, "forbidden")
			expectError(t, e.api.Patch("/rewards/challenges/"+id, caller.bearer, map[string]any{}), http.StatusForbidden, "forbidden")
			expectError(t, e.api.Delete("/rewards/challenges/"+id, caller.bearer), http.StatusForbidden, "forbidden")
		}
		for _, rec := range []int{
			e.api.Get("/rewards/challenges").Code,
			e.api.Get("/rewards/challenges/" + id).Code,
			e.api.Get("/rewards/me/challenges").Code,
		} {
			if rec != http.StatusUnauthorized {
				t.Errorf("without a session got %d, want 401", rec)
			}
		}
	})
}

func TestTrackChallenges(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("reaching the target pays the reward once per period", func(t *testing.T) {
		admin, student := e.admin(t), e.user(t)
		c := e.challenge(t, admin, nil)

		var paid []int
		for range 3 {
			coins, err := e.module.Track(ctx, student.id, ActionProfessorRated)
			if err != nil {
				t.Fatal(err)
			}
			paid = append(paid, coins)
		}
		if paid[0] != 0 || paid[1] < 15 || paid[2] != 0 {
			t.Errorf("paid = %v, want the reward on the second action only", paid)
		}
		if progress, completed := e.progressOf(t, student.id, c.ID); progress != 2 || !completed {
			t.Errorf("progress = %d, completed = %v", progress, completed)
		}
	})

	t.Run("actions at the same moment complete a challenge once", func(t *testing.T) {
		admin, student := e.admin(t), e.user(t)
		c := e.challenge(t, admin, map[string]any{"target": 3, "period": "once", "reward": 7})

		var wg sync.WaitGroup
		for range 10 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := e.module.Track(ctx, student.id, ActionProfessorRated); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()

		var grants int
		if err := e.pool.QueryRow(ctx, `
			SELECT count(*) FROM coin_grants g JOIN challenge_progress p ON p.id = g.ref_id
			WHERE g.user_id = $1 AND g.reason = $2 AND p.challenge_id = $3`, student.id, ReasonChallenge, c.ID).Scan(&grants); err != nil {
			t.Fatal(err)
		}
		if progress, completed := e.progressOf(t, student.id, c.ID); grants != 1 || progress != 3 || !completed {
			t.Errorf("grants = %d, progress = %d, completed = %v; want one grant at progress 3", grants, progress, completed)
		}
	})

	t.Run("archived, future and ended challenges do not count", func(t *testing.T) {
		admin, student := e.admin(t), e.user(t)
		now := time.Now().UTC()
		archived := e.challenge(t, admin, map[string]any{"target": unreachable})
		if rec := e.api.Delete("/rewards/challenges/"+archived.ID.String(), admin.bearer); rec.Code != http.StatusNoContent {
			t.Fatal(rec.Body.String())
		}
		future := e.challenge(t, admin, map[string]any{"target": unreachable, "startsAt": now.Add(time.Hour)})
		ended := e.challenge(t, admin, map[string]any{"target": unreachable, "startsAt": now.Add(-2 * time.Hour), "endsAt": now.Add(-time.Hour)})

		if _, err := e.module.Track(ctx, student.id, ActionProfessorRated); err != nil {
			t.Fatal(err)
		}
		for _, c := range []Challenge{archived, future, ended} {
			if progress, _ := e.progressOf(t, student.id, c.ID); progress != 0 {
				t.Errorf("challenge %s counted %d", c.ID, progress)
			}
		}
	})

	t.Run("a zero reward completes without paying", func(t *testing.T) {
		admin, student := e.admin(t), e.user(t)
		c := e.challenge(t, admin, map[string]any{"target": 1, "period": "once", "reward": 0})

		if _, err := e.module.Track(ctx, student.id, ActionProfessorRated); err != nil {
			t.Fatal(err)
		}
		if _, completed := e.progressOf(t, student.id, c.ID); !completed {
			t.Error("challenge not completed")
		}
		if coins := e.challengeCoins(t, student.id); coins != 0 {
			t.Errorf("paid %d coins", coins)
		}
	})

	t.Run("a paid grant counts its action, and a repeated one does not", func(t *testing.T) {
		admin, student := e.admin(t), e.user(t)
		c := e.challenge(t, admin, map[string]any{"action": string(ActionFAQApproved), "target": unreachable})
		entry := uuid.New()

		for range 2 {
			if _, err := e.module.Grant(ctx, student.id, ReasonFAQApproved, &entry, nil); err != nil {
				t.Fatal(err)
			}
		}
		if progress, _ := e.progressOf(t, student.id, c.ID); progress != 1 {
			t.Errorf("progress = %d, want 1", progress)
		}
	})

	t.Run("a post counts towards post challenges", func(t *testing.T) {
		admin, student := e.admin(t), e.user(t)
		c := e.challenge(t, admin, map[string]any{"action": string(ActionPostCreated), "target": unreachable})

		if _, err := e.module.Progress(ctx, student.id, QuestPost); err != nil {
			t.Fatal(err)
		}
		if progress, _ := e.progressOf(t, student.id, c.ID); progress != 1 {
			t.Errorf("progress = %d, want 1", progress)
		}
	})
}

func TestListMyChallenges(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	admin, student := e.admin(t), e.user(t)
	daily := e.challenge(t, admin, map[string]any{"target": unreachable})
	once := e.challenge(t, admin, map[string]any{"target": unreachable, "period": "once"})
	archived := e.challenge(t, admin, map[string]any{"target": unreachable})
	if rec := e.api.Delete("/rewards/challenges/"+archived.ID.String(), admin.bearer); rec.Code != http.StatusNoContent {
		t.Fatal(rec.Body.String())
	}
	if _, err := e.module.Track(ctx, student.id, ActionProfessorRated); err != nil {
		t.Fatal(err)
	}

	mine := map[uuid.UUID]MyChallenge{}
	cursor := ""
	for {
		path := "/rewards/me/challenges?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		page := decode[struct {
			Items      []MyChallenge `json:"items"`
			NextCursor *string       `json:"nextCursor"`
		}](t, e.api.Get(path, student.bearer), http.StatusOK)
		for _, item := range page.Items {
			mine[item.ID] = item
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}

	if got, ok := mine[daily.ID]; !ok || got.Progress != 1 || got.Completed || got.ResetsAt == nil || got.Texts["pl"].Title == "" {
		t.Errorf("daily = %+v, listed %v", got, ok)
	}
	if got, ok := mine[once.ID]; !ok || got.Progress != 1 || got.ResetsAt != nil {
		t.Errorf("once = %+v, listed %v", got, ok)
	}
	if _, ok := mine[archived.ID]; ok {
		t.Error("archived challenge is listed")
	}
}

func TestPruneChallengeProgress(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	admin, student := e.admin(t), e.user(t)
	daily := e.challenge(t, admin, map[string]any{"target": unreachable})
	once := e.challenge(t, admin, map[string]any{"target": unreachable, "period": "once"})

	if _, err := e.pool.Exec(ctx, `
		INSERT INTO challenge_progress (user_id, challenge_id, period_start, target, progress)
		VALUES ($1, $2, CURRENT_DATE - 40, $4, 1), ($1, $2, CURRENT_DATE, $4, 1), ($1, $3, $5, $4, 1)`,
		student.id, daily.ID, once.ID, unreachable, onceStart); err != nil {
		t.Fatal(err)
	}

	if err := e.module.pruneQuests(ctx, PruneQuestsArgs{}); err != nil {
		t.Fatal(err)
	}
	if progress, _ := e.progressOf(t, student.id, daily.ID); progress != 1 {
		t.Errorf("daily progress rows left = %d, want only today's", progress)
	}
	if progress, _ := e.progressOf(t, student.id, once.ID); progress != 1 {
		t.Errorf("once progress was pruned")
	}
}
