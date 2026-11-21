package study

import (
	"context"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/google/uuid"
)

func (e *testEnv) challenge(t *testing.T, action rewards.Action) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := e.pool.QueryRow(context.Background(), `
		INSERT INTO challenges (action, target, period, reward, texts)
		VALUES ($1, 1000, 'once', 0, '{"pl": {"title": "Test"}}') RETURNING id`, action).Scan(&id); err != nil {
		t.Fatalf("insert challenge: %v", err)
	}
	t.Cleanup(func() { _, _ = e.pool.Exec(context.Background(), "DELETE FROM challenges WHERE id = $1", id) })
	return id
}

func (e *testEnv) challengeProgress(t *testing.T, userID, challengeID uuid.UUID) int {
	t.Helper()
	var progress int
	if err := e.pool.QueryRow(context.Background(),
		"SELECT COALESCE(SUM(progress), 0)::int FROM challenge_progress WHERE user_id = $1 AND challenge_id = $2", userID, challengeID).Scan(&progress); err != nil {
		t.Fatal(err)
	}
	return progress
}

func TestChallengeActions(t *testing.T) {
	e := newTestEnv(t)

	t.Run("an uploaded file counts", func(t *testing.T) {
		challenge := e.challenge(t, rewards.ActionFileUploaded)
		owner := e.student(t)
		course := e.course(t, owner, nil)

		e.file(t, owner, course.ID, "Final exam")
		if got := e.challengeProgress(t, owner.id, challenge); got != 1 {
			t.Errorf("progress = %d, want 1", got)
		}
	})

	t.Run("rating a course counts once, however often the rating changes", func(t *testing.T) {
		challenge := e.challenge(t, rewards.ActionCourseRated)
		owner := e.student(t)
		course := e.course(t, owner, nil)

		for _, rating := range []int{2, 5} {
			decode[Course](t, e.api.Put("/study/courses/"+course.ID.String()+"/rating", owner.bearer, map[string]any{"rating": rating}), http.StatusOK)
		}
		if got := e.challengeProgress(t, owner.id, challenge); got != 1 {
			t.Errorf("progress = %d, want 1", got)
		}
	})
}
