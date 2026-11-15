package rewards

import (
	"context"
	"testing"
)

func TestPruneQuests(t *testing.T) {
	pool := setupTestDB(t)
	m := newModule(t, pool)
	ctx := context.Background()

	t.Run("removes progress older than 30 days and keeps the rest", func(t *testing.T) {
		user := newUser(t, pool)
		_, err := pool.Exec(ctx, `
			INSERT INTO daily_quests (user_id, day)
			SELECT $1, (NOW() AT TIME ZONE $2)::date - d FROM unnest(ARRAY[0, 30, 31]) AS d`, user, questZone)
		if err != nil {
			t.Fatalf("insert quests: %v", err)
		}

		for range 2 {
			if err := m.pruneQuests(ctx, PruneQuestsArgs{}); err != nil {
				t.Fatalf("prune: %v", err)
			}
		}

		var ages []int
		rows, err := pool.Query(ctx, `SELECT (NOW() AT TIME ZONE $2)::date - day FROM daily_quests WHERE user_id = $1 ORDER BY 1`, user, questZone)
		if err != nil {
			t.Fatalf("read quests: %v", err)
		}
		defer rows.Close()
		for rows.Next() {
			var age int
			if err := rows.Scan(&age); err != nil {
				t.Fatalf("scan: %v", err)
			}
			ages = append(ages, age)
		}
		if len(ages) != 2 || ages[0] != 0 || ages[1] != 30 {
			t.Errorf("days left = %v, want [0 30]", ages)
		}
	})
}
