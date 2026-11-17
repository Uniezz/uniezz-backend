package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recordingReferrals struct {
	mu      sync.Mutex
	rewards [][2]uuid.UUID
}

func (r *recordingReferrals) RewardReferral(_ context.Context, userID, inviterID uuid.UUID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rewards = append(r.rewards, [2]uuid.UUID{userID, inviterID})
	return 50, nil
}

func newInviteTestModule(pool *pgxpool.Pool) (Module, *UserRepository, *recordingReferrals) {
	db := database.New(pool)
	users := NewUserRepository(db)
	referrals := &recordingReferrals{}
	l := newLogins(db, users, NewSessionRepository(db), newLoginCodeRepository(db), time.Hour)
	l.referrals = referrals
	return Module{logins: l}, users, referrals
}

func invitedBy(t *testing.T, pool *pgxpool.Pool, userID uuid.UUID) *uuid.UUID {
	t.Helper()
	var inviterID *uuid.UUID
	if err := pool.QueryRow(context.Background(), "SELECT invited_by FROM users WHERE id = $1", userID).Scan(&inviterID); err != nil {
		t.Fatalf("read invited_by: %v", err)
	}
	return inviterID
}

func TestRedeemInvite(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	newUser := func(t *testing.T, users *UserRepository) *User {
		return upsertTestUser(t, pool, users, Identity{UniversityID: UniversityUMCS, UsosUserID: testUsosUserID()})
	}

	t.Run("a new account without an inviter gets one and both are rewarded", func(t *testing.T) {
		m, users, referrals := newInviteTestModule(pool)
		inviter, student := newUser(t, users), newUser(t, users)

		earned, err := m.redeemInvite(ctx, student.ID, inviteChange{Code: " " + inviter.InviteCode + " "})
		if err != nil {
			t.Fatalf("redeem: %v", err)
		}
		if earned != 50 {
			t.Errorf("earned = %d, want 50", earned)
		}
		if got := invitedBy(t, pool, student.ID); got == nil || *got != inviter.ID {
			t.Errorf("invited_by = %v, want %s", got, inviter.ID)
		}
		if len(referrals.rewards) != 1 || referrals.rewards[0] != [2]uuid.UUID{student.ID, inviter.ID} {
			t.Errorf("rewards = %v", referrals.rewards)
		}
	})

	t.Run("sending the applied code again changes nothing", func(t *testing.T) {
		m, users, referrals := newInviteTestModule(pool)
		inviter, student := newUser(t, users), newUser(t, users)

		if _, err := m.redeemInvite(ctx, student.ID, inviteChange{Code: inviter.InviteCode}); err != nil {
			t.Fatalf("first redeem: %v", err)
		}
		earned, err := m.redeemInvite(ctx, student.ID, inviteChange{Code: inviter.InviteCode})
		if err != nil || earned != 0 {
			t.Errorf("second redeem = %d, %v; want 0, nil", earned, err)
		}
		if len(referrals.rewards) != 1 {
			t.Errorf("expected one reward, got %v", referrals.rewards)
		}
	})

	t.Run("a second inviter is rejected", func(t *testing.T) {
		m, users, _ := newInviteTestModule(pool)
		first, second, student := newUser(t, users), newUser(t, users), newUser(t, users)

		if _, err := m.redeemInvite(ctx, student.ID, inviteChange{Code: first.InviteCode}); err != nil {
			t.Fatalf("first redeem: %v", err)
		}
		if _, err := m.redeemInvite(ctx, student.ID, inviteChange{Code: second.InviteCode}); !errors.Is(err, ErrReferralAlreadySet) {
			t.Errorf("expected ErrReferralAlreadySet, got %v", err)
		}
	})

	t.Run("an account older than the window is rejected", func(t *testing.T) {
		m, users, referrals := newInviteTestModule(pool)
		inviter, student := newUser(t, users), newUser(t, users)
		if _, err := pool.Exec(ctx, "UPDATE users SET created_at = NOW() - $2::interval - INTERVAL '1 minute' WHERE id = $1", student.ID, inviteRedeemWindow); err != nil {
			t.Fatal(err)
		}

		if _, err := m.redeemInvite(ctx, student.ID, inviteChange{Code: inviter.InviteCode}); !errors.Is(err, ErrReferralWindowClosed) {
			t.Errorf("expected ErrReferralWindowClosed, got %v", err)
		}
		if len(referrals.rewards) != 0 {
			t.Errorf("expected no rewards, got %v", referrals.rewards)
		}
	})

	t.Run("own, unknown, malformed and reverse codes are invalid", func(t *testing.T) {
		m, users, referrals := newInviteTestModule(pool)
		inviter, student := newUser(t, users), newUser(t, users)
		if _, err := m.redeemInvite(ctx, student.ID, inviteChange{Code: inviter.InviteCode}); err != nil {
			t.Fatalf("redeem: %v", err)
		}
		loner := newUser(t, users)

		for _, tc := range []struct {
			name   string
			userID uuid.UUID
			code   string
		}{
			{"own code", loner.ID, loner.InviteCode},
			{"unknown code", loner.ID, "ZZZZZZZ"},
			{"malformed code", loner.ID, "abc"},
			{"the code of a student you invited", inviter.ID, student.InviteCode},
		} {
			if _, err := m.redeemInvite(ctx, tc.userID, inviteChange{Code: tc.code}); !errors.Is(err, ErrInvalidReferralCode) {
				t.Errorf("%s: expected ErrInvalidReferralCode, got %v", tc.name, err)
			}
		}
		if len(referrals.rewards) != 1 {
			t.Errorf("expected only the first reward, got %v", referrals.rewards)
		}
	})

	t.Run("two codes redeemed at once leave exactly one inviter", func(t *testing.T) {
		m, users, referrals := newInviteTestModule(pool)
		first, second, student := newUser(t, users), newUser(t, users), newUser(t, users)

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, code := range []string{first.InviteCode, second.InviteCode} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = m.redeemInvite(ctx, student.ID, inviteChange{Code: code})
			}()
		}
		wg.Wait()

		failed := 0
		for _, err := range errs {
			if errors.Is(err, ErrReferralAlreadySet) {
				failed++
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}
		if failed != 1 || len(referrals.rewards) != 1 {
			t.Errorf("expected one success and one conflict, got errors %v and rewards %v", errs, referrals.rewards)
		}
		if got := invitedBy(t, pool, student.ID); got == nil || *got != referrals.rewards[0][1] {
			t.Errorf("invited_by = %v, rewards = %v", got, referrals.rewards)
		}
	})
}
