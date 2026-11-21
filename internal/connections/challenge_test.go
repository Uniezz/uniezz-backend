package connections

import (
	"context"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/google/uuid"
)

func TestNewConnectionCountsForBoth(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	var challenge uuid.UUID
	if err := e.pool.QueryRow(ctx, `
		INSERT INTO challenges (action, target, period, reward, texts)
		VALUES ($1, 1000, 'once', 0, '{"pl": {"title": "Test"}}') RETURNING id`, rewards.ActionConnectionMade).Scan(&challenge); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM challenges WHERE id = $1", challenge) })

	a, b := e.user(t), e.user(t)
	e.match(t, a, b)

	for _, u := range []testUser{a, b} {
		var progress int
		if err := e.pool.QueryRow(ctx,
			"SELECT COALESCE(SUM(progress), 0)::int FROM challenge_progress WHERE user_id = $1 AND challenge_id = $2", u.id, challenge).Scan(&progress); err != nil {
			t.Fatal(err)
		}
		if progress != 1 {
			t.Errorf("progress of %s = %d, want 1", u.id, progress)
		}
	}
}
