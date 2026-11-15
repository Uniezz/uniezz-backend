package rewards

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

type grantPage struct {
	Items      []CoinGrant `json:"items"`
	NextCursor *string     `json:"nextCursor"`
}

func TestGetMyDailyQuests(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("a student with no progress today sees every quest at zero", func(t *testing.T) {
		u := e.user(t)
		got := decode[DailyQuests](t, e.api.Get("/rewards/me/quests", u.bearer), http.StatusOK)
		if len(got.Quests) != 3 || got.Completed || got.Reward != 10 || !got.ResetsAt.After(time.Now()) {
			t.Fatalf("quests = %+v", got)
		}
		for _, q := range got.Quests {
			if q.Progress != 0 || q.Done {
				t.Errorf("quest %s = %+v, want no progress", q.Quest, q)
			}
		}
		if _, err := time.Parse(time.DateOnly, got.Day); err != nil {
			t.Errorf("day %q: %v", got.Day, err)
		}
	})

	t.Run("progress shows per quest and completed flips once the reward is paid", func(t *testing.T) {
		u := e.user(t)
		for _, q := range []Quest{QuestLike, QuestLike, QuestPost} {
			if err := e.module.Progress(ctx, u.id, q); err != nil {
				t.Fatalf("progress: %v", err)
			}
		}
		got := decode[DailyQuests](t, e.api.Get("/rewards/me/quests", u.bearer), http.StatusOK)
		want := map[Quest]QuestProgress{
			QuestLike:  {Quest: QuestLike, Progress: 2, Target: 3},
			QuestReply: {Quest: QuestReply, Progress: 0, Target: 1},
			QuestPost:  {Quest: QuestPost, Progress: 1, Target: 1, Done: true},
		}
		for _, q := range got.Quests {
			if q != want[q.Quest] {
				t.Errorf("quest %s = %+v, want %+v", q.Quest, q, want[q.Quest])
			}
		}
		if got.Completed {
			t.Error("completed = true before every quest is done")
		}

		for _, q := range []Quest{QuestLike, QuestLike, QuestReply} {
			if err := e.module.Progress(ctx, u.id, q); err != nil {
				t.Fatalf("progress: %v", err)
			}
		}
		got = decode[DailyQuests](t, e.api.Get("/rewards/me/quests", u.bearer), http.StatusOK)
		if !got.Completed || got.Quests[0].Progress != 3 {
			t.Errorf("after finishing = %+v", got)
		}
	})

	t.Run("without a session it is a 401", func(t *testing.T) {
		expectError(t, e.api.Get("/rewards/me/quests"), http.StatusUnauthorized, "unauthorized")
	})
}

func TestListMyCoinGrants(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("pages through the student's own grants, newest first", func(t *testing.T) {
		u, other := e.user(t), e.user(t)
		for _, r := range []Reason{ReasonWelcome, ReasonBirthDate, ReasonProfileComplete} {
			if _, err := e.module.Grant(ctx, u.id, r, nil, nil); err != nil {
				t.Fatalf("grant: %v", err)
			}
		}
		if _, err := e.module.Grant(ctx, other.id, ReasonWelcome, nil, nil); err != nil {
			t.Fatalf("grant: %v", err)
		}

		var reasons []Reason
		cursor := ""
		for range 10 {
			page := decode[grantPage](t, e.api.Get("/rewards/me/grants?limit=2"+cursor, u.bearer), http.StatusOK)
			for _, g := range page.Items {
				reasons = append(reasons, g.Reason)
			}
			if page.NextCursor == nil {
				break
			}
			cursor = "&cursor=" + url.QueryEscape(*page.NextCursor)
		}
		want := []Reason{ReasonProfileComplete, ReasonBirthDate, ReasonWelcome}
		if len(reasons) != len(want) {
			t.Fatalf("reasons = %v, want %v", reasons, want)
		}
		for i := range want {
			if reasons[i] != want[i] {
				t.Errorf("reasons = %v, want %v", reasons, want)
				break
			}
		}
	})

	t.Run("a student with no grants gets an empty list", func(t *testing.T) {
		u := e.user(t)
		page := decode[grantPage](t, e.api.Get("/rewards/me/grants", u.bearer), http.StatusOK)
		if page.Items == nil || len(page.Items) != 0 || page.NextCursor != nil {
			t.Errorf("page = %+v", page)
		}
	})

	t.Run("a broken cursor is a 400", func(t *testing.T) {
		u := e.user(t)
		expectError(t, e.api.Get("/rewards/me/grants?cursor=nope", u.bearer), http.StatusBadRequest, "invalid_cursor")
	})

	t.Run("without a session it is a 401", func(t *testing.T) {
		expectError(t, e.api.Get("/rewards/me/grants"), http.StatusUnauthorized, "unauthorized")
	})
}
