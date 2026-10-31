package profile

import (
	"context"
	"encoding/json"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
)

func TestInitProfile(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.otpUser(t)

	var afterCommit func()
	err := database.New(e.pool).WithTx(ctx, func(ctx context.Context) error {
		var err error
		afterCommit, err = e.module.InitProfile(ctx, u.id, auth.InitialProfile{PhotoURL: "https://usos.test/photo.jpg"})
		return err
	})
	if err != nil {
		t.Fatalf("InitProfile failed: %v", err)
	}
	if afterCommit == nil {
		t.Error("expected a wakeup to run after commit")
	}

	var payload ImportAvatarArgs
	var raw []byte
	err = e.pool.QueryRow(ctx, "SELECT payload FROM jobs WHERE kind = $1 AND payload->>'userId' = $2", ImportAvatarArgs{}.Kind(), u.id.String()).Scan(&raw)
	if err != nil {
		t.Fatalf("expected an import job: %v", err)
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.SourceURL != "https://usos.test/photo.jpg" {
		t.Errorf("unexpected payload %s", raw)
	}
}
