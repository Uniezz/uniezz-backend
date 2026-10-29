package profile

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/jackc/pgx/v5"
)

func TestSeedAccount(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)

	var afterCommit func()
	err := pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		afterCommit, err = e.module.SeedAccount(ctx, tx, u.id, auth.AccountSeed{PhotoURL: "https://usos.test/photo.jpg"})
		return err
	})
	if err != nil {
		t.Fatalf("SeedAccount failed: %v", err)
	}
	if afterCommit == nil {
		t.Error("expected a wakeup to run after commit")
	}

	var payload ImportAvatarArgs
	var raw []byte
	err = e.pool.QueryRow(ctx, "SELECT payload FROM jobs WHERE kind = $1 AND payload->>'userId' = $2", ImportAvatar.Kind(), u.id.String()).Scan(&raw)
	if err != nil {
		t.Fatalf("expected an import job: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.SourceURL != "https://usos.test/photo.jpg" {
		t.Errorf("unexpected payload %s", raw)
	}
}
