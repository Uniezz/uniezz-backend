package push

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func (e *testEnv) queued(t *testing.T, marker uuid.UUID) []SendArgs {
	t.Helper()
	rows, err := e.pool.Query(context.Background(),
		"SELECT payload::text FROM jobs WHERE kind = $1 AND payload::text LIKE '%' || $2 || '%' ORDER BY created_at, id", SendArgs{}.Kind(), marker.String())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []SendArgs
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			t.Fatal(err)
		}
		out = append(out, SendArgs{Preview: payload})
	}
	t.Cleanup(func() {
		_, _ = e.pool.Exec(context.Background(), "DELETE FROM jobs WHERE kind = $1 AND payload::text LIKE '%' || $2 || '%'", SendArgs{}.Kind(), marker.String())
	})
	return out
}

func TestNotify(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("a burst in one conversation sends one push per recipient", func(t *testing.T) {
		sender, a, b := e.user(t, "pl"), e.user(t, "pl"), e.user(t, "pl")
		conversationID := uuid.New()
		args := SendArgs{Event: EventMessage, RecipientIDs: []uuid.UUID{a.id}, ActorID: sender.id, ConversationID: &conversationID, Preview: "one"}

		if err := e.module.Notify(ctx, args); err != nil {
			t.Fatal(err)
		}
		args.Preview = "two"
		if err := e.module.Notify(ctx, args); err != nil {
			t.Fatal(err)
		}
		args.RecipientIDs = []uuid.UUID{a.id, b.id}
		if err := e.module.Notify(ctx, args); err != nil {
			t.Fatal(err)
		}

		jobs := e.queued(t, conversationID)
		if len(jobs) != 2 {
			t.Fatalf("queued %d jobs, want 2", len(jobs))
		}
		if strings.Contains(jobs[1].Preview, a.id.String()) {
			t.Errorf("second job still targets a: %s", jobs[1].Preview)
		}
	})

	t.Run("matches and requests are never collapsed, and previews are cut short", func(t *testing.T) {
		sender, recipient := e.user(t, "pl"), e.user(t, "pl")
		for range 2 {
			if err := e.module.Notify(ctx, SendArgs{Event: EventMatch, RecipientIDs: []uuid.UUID{recipient.id}, ActorID: sender.id, Preview: strings.Repeat("x", 500)}); err != nil {
				t.Fatal(err)
			}
		}
		jobs := e.queued(t, recipient.id)
		if len(jobs) != 2 {
			t.Fatalf("queued %d match jobs, want 2", len(jobs))
		}
		if strings.Contains(jobs[0].Preview, strings.Repeat("x", maxPreviewLength+1)) {
			t.Errorf("preview was not shortened")
		}
	})
}
