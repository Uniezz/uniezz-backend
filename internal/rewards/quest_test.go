package rewards

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func dailyRows(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) (rows, grants int) {
	t.Helper()
	err := pool.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM daily_quests WHERE user_id = $1),
			(SELECT count(*) FROM coin_grants WHERE user_id = $1 AND reason = 'daily_quests')`, userID).Scan(&rows, &grants)
	if err != nil {
		t.Fatalf("count daily rows: %v", err)
	}
	return rows, grants
}

func TestProgress(t *testing.T) {
	pool := setupTestDB(t)
	m := New(pool)
	ctx := context.Background()
	progress := func(t *testing.T, user uuid.UUID, quests ...Quest) {
		t.Helper()
		for _, q := range quests {
			if err := m.Progress(ctx, user, q); err != nil {
				t.Fatalf("progress %s: %v", q, err)
			}
		}
	}
	allQuests := []Quest{QuestLike, QuestLike, QuestLike, QuestReply, QuestPost}

	t.Run("finishing every quest pays 10 coins", func(t *testing.T) {
		user := newUser(t, pool)
		progress(t, user, allQuests...)
		if got, _ := m.Balance(ctx, user); got != 10 {
			t.Errorf("balance = %d, want 10", got)
		}
	})

	t.Run("doing the quests again the same day pays nothing more", func(t *testing.T) {
		user := newUser(t, pool)
		progress(t, user, allQuests...)
		progress(t, user, allQuests...)
		rows, grants := dailyRows(t, pool, user)
		if got, _ := m.Balance(ctx, user); got != 10 || rows != 1 || grants != 1 {
			t.Errorf("balance %d, rows %d, grants %d; want 10, 1, 1", got, rows, grants)
		}
	})

	t.Run("two likes are not enough", func(t *testing.T) {
		user := newUser(t, pool)
		progress(t, user, QuestLike, QuestLike, QuestReply, QuestPost, QuestReply, QuestPost)
		if got, _ := m.Balance(ctx, user); got != 0 {
			t.Errorf("balance = %d, want 0", got)
		}
	})

	t.Run("progress from yesterday does not count today", func(t *testing.T) {
		user := newUser(t, pool)
		_, err := pool.Exec(ctx, `
			INSERT INTO daily_quests (user_id, day, likes, replied, posted)
			VALUES ($1, (NOW() AT TIME ZONE $2)::date - 1, 3, TRUE, FALSE)`, user, questZone)
		if err != nil {
			t.Fatalf("insert yesterday: %v", err)
		}
		progress(t, user, QuestPost)
		rows, _ := dailyRows(t, pool, user)
		if got, _ := m.Balance(ctx, user); got != 0 || rows != 2 {
			t.Errorf("balance %d, rows %d; want 0, 2", got, rows)
		}
	})

	t.Run("concurrent actions finish the day once", func(t *testing.T) {
		user := newUser(t, pool)
		progress(t, user, QuestLike, QuestLike, QuestReply, QuestPost)
		var wg sync.WaitGroup
		errs := make([]error, 10)
		for i := range errs {
			wg.Go(func() { errs[i] = m.Progress(ctx, user, QuestLike) })
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			t.Fatalf("progress: %v", err)
		}
		var likes int
		if err := pool.QueryRow(ctx, "SELECT likes FROM daily_quests WHERE user_id = $1", user).Scan(&likes); err != nil {
			t.Fatalf("read likes: %v", err)
		}
		_, grants := dailyRows(t, pool, user)
		if got, _ := m.Balance(ctx, user); got != 10 || grants != 1 || likes != 12 {
			t.Errorf("balance %d, grants %d, likes %d; want 10, 1, 12", got, grants, likes)
		}
	})

	t.Run("an unknown quest is an error and records nothing", func(t *testing.T) {
		user := newUser(t, pool)
		if err := m.Progress(ctx, user, Quest("bogus")); err == nil {
			t.Error("want an error")
		}
		if rows, _ := dailyRows(t, pool, user); rows != 0 {
			t.Errorf("rows = %d, want 0", rows)
		}
	})
}
