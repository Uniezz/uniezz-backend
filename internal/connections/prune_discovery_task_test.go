package connections

import (
	"context"
	"testing"
)

func TestPruneDiscoveryKeepsRecentRows(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	viewer := e.user(t)
	old := e.user(t)
	recent := e.user(t)

	_, err := e.pool.Exec(ctx, `
		INSERT INTO discovery_candidates (viewer_id, user_id, created_at)
		VALUES ($1, $2, NOW() - INTERVAL '31 days'), ($1, $3, NOW())`, viewer.id, old.id, recent.id)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.pool.Exec(ctx, `
		INSERT INTO connection_actions (viewer_id, target_id, mode, action, created_at)
		VALUES ($1, $2, 'friends', 'pass', NOW() - INTERVAL '31 days'),
			($1, $3, 'friends', 'pass', NOW()),
			($1, $2, 'friends', 'like', NOW() - INTERVAL '31 days')`, viewer.id, old.id, recent.id)
	if err != nil {
		t.Fatal(err)
	}

	module, err := New(e.pool, nil, newRunner(e.pool))
	if err != nil {
		t.Fatal(err)
	}
	if err := module.pruneDiscovery(ctx, PruneDiscoveryArgs{}); err != nil {
		t.Fatal(err)
	}

	var candidates, passes, likes int
	err = e.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM discovery_candidates WHERE viewer_id = $1),
			(SELECT count(*) FROM connection_actions WHERE viewer_id = $1 AND action = 'pass'),
			(SELECT count(*) FROM connection_actions WHERE viewer_id = $1 AND action = 'like')`, viewer.id).Scan(&candidates, &passes, &likes)
	if err != nil {
		t.Fatal(err)
	}
	if candidates != 1 || passes != 1 || likes != 1 {
		t.Errorf("candidates = %d, passes = %d, likes = %d, want 1 each", candidates, passes, likes)
	}
}
