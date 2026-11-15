package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
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

	if got := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK).Coins; got != 100 {
		t.Errorf("coins after the first init = %d, want 100", got)
	}

	err = database.New(e.pool).WithTx(ctx, func(ctx context.Context) error {
		_, err := e.module.InitProfile(ctx, u.id, auth.InitialProfile{})
		return err
	})
	if err != nil {
		t.Fatalf("second InitProfile failed: %v", err)
	}
	if got := decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK).Coins; got != 100 {
		t.Errorf("coins after a second init = %d, want 100", got)
	}
}

func TestInitProfileAwardsReferralCoins(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	inviter, invited := e.otpUser(t), e.otpUser(t)

	err := database.New(e.pool).WithTx(ctx, func(ctx context.Context) error {
		_, err := e.module.InitProfile(ctx, invited.id, auth.InitialProfile{InviterID: &inviter.id})
		return err
	})
	if err != nil {
		t.Fatalf("InitProfile: %v", err)
	}

	for _, tc := range []struct {
		user uuid.UUID
		want int
	}{{inviter.id, 100}, {invited.id, 150}} {
		got, err := e.module.rewards.Balance(ctx, tc.user)
		if err != nil || got != tc.want {
			t.Errorf("balance for %s = %d, %v; want %d", tc.user, got, err, tc.want)
		}
	}
}
