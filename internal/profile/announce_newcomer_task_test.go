package profile

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAnnounceNewcomer(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	t.Run("posts the name, university and place among onboarded students", func(t *testing.T) {
		u := e.newUser(t, "umcs", "newcomer-"+uuid.NewString()+"@umcs.pl")
		if _, err := e.pool.Exec(ctx, "UPDATE users SET first_name = 'Anna', last_name = 'Kowalska' WHERE id = $1", u.id); err != nil {
			t.Fatal(err)
		}
		var place int
		err := e.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM users o, users u
			WHERE u.id = $1 AND o.onboarded_at IS NOT NULL AND o.deleted_at IS NULL AND o.banned_at IS NULL
			  AND (o.onboarded_at, o.id) <= (u.onboarded_at, u.id)`, u.id).Scan(&place)
		if err != nil {
			t.Fatal(err)
		}
		before := len(e.newcomers.sent())

		if err := e.module.announceNewcomer(ctx, AnnounceNewcomerArgs{UserID: u.id}); err != nil {
			t.Fatalf("announceNewcomer: %v", err)
		}

		sent := e.newcomers.sent()[before:]
		want := fmt.Sprintf("👋 New student: Anna Kowalska · UMCS · #%d", place)
		if len(sent) != 1 || sent[0] != want {
			t.Errorf("sent = %q, want [%q]", sent, want)
		}
	})

	t.Run("a student without a name is still announced", func(t *testing.T) {
		u := e.otpUser(t)
		if _, err := e.pool.Exec(ctx, "UPDATE users SET first_name = NULL, last_name = NULL WHERE id = $1", u.id); err != nil {
			t.Fatal(err)
		}
		before := len(e.newcomers.sent())

		if err := e.module.announceNewcomer(ctx, AnnounceNewcomerArgs{UserID: u.id}); err != nil {
			t.Fatalf("announceNewcomer: %v", err)
		}

		sent := e.newcomers.sent()[before:]
		if len(sent) != 1 || !strings.Contains(sent[0], "(no name yet) · KUL") {
			t.Errorf("sent = %q", sent)
		}
	})

	t.Run("a student deleted before the job runs is not announced", func(t *testing.T) {
		u := e.otpUser(t)
		if _, err := e.pool.Exec(ctx, "UPDATE users SET deleted_at = NOW() WHERE id = $1", u.id); err != nil {
			t.Fatal(err)
		}
		before := len(e.newcomers.sent())

		if err := e.module.announceNewcomer(ctx, AnnounceNewcomerArgs{UserID: u.id}); err != nil {
			t.Fatalf("announceNewcomer: %v", err)
		}
		if sent := e.newcomers.sent()[before:]; len(sent) != 0 {
			t.Errorf("sent = %q, want none", sent)
		}
	})

	t.Run("a failed send is returned so the job retries", func(t *testing.T) {
		u := e.otpUser(t)
		failure := errors.New("telegram is down")
		e.newcomers.mu.Lock()
		e.newcomers.err = failure
		e.newcomers.mu.Unlock()
		t.Cleanup(func() {
			e.newcomers.mu.Lock()
			e.newcomers.err = nil
			e.newcomers.mu.Unlock()
		})

		if err := e.module.announceNewcomer(ctx, AnnounceNewcomerArgs{UserID: u.id}); !errors.Is(err, failure) {
			t.Errorf("err = %v, want %v", err, failure)
		}
	})
}
