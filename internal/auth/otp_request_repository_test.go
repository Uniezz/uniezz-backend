package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var testOTPHasher = newOTPHasher([]byte("test-key"))

func newTestOTPEmail() string {
	return "otp-" + uuid.NewString() + "@student.kul.pl"
}

func mustCreateOTP(t *testing.T, repo *otpRequestRepository, email, code string) {
	t.Helper()
	req := otpRequest{UniversityID: UniversityKUL, Email: email, CodeHash: testOTPHasher.hash(code)}
	if err := repo.create(context.Background(), req, 10*time.Minute); err != nil {
		t.Fatalf("create failed: %v", err)
	}
}

func verifyOTP(repo *otpRequestRepository, email, code string) error {
	return repo.verify(context.Background(), UniversityKUL, email, testOTPHasher.hash(code))
}

func execTest(t *testing.T, pool *pgxpool.Pool, query string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec failed: %v", err)
	}
}

func TestOTPRequestRepository_Integration(t *testing.T) {
	pool := setupTestDB(t)
	repo := newOTPRequestRepository(pool)

	t.Run("correct code verifies", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		if err := verifyOTP(repo, email, "123456"); err != nil {
			t.Fatalf("expected the code to verify, got: %v", err)
		}
	})

	t.Run("email lookup ignores case", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		if err := verifyOTP(repo, "OTP"+email[3:], "123456"); err != nil {
			t.Fatalf("expected a case-insensitive match, got: %v", err)
		}
	})

	t.Run("code works only once", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		if err := verifyOTP(repo, email, "123456"); err != nil {
			t.Fatal(err)
		}
		if err := verifyOTP(repo, email, "123456"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode on reuse, got: %v", err)
		}
	})

	t.Run("wrong code is rejected but the right one still works", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		if err := verifyOTP(repo, email, "000000"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode, got: %v", err)
		}
		if err := verifyOTP(repo, email, "123456"); err != nil {
			t.Fatalf("expected the right code to verify after a wrong one, got: %v", err)
		}
	})

	t.Run("no request", func(t *testing.T) {
		if err := verifyOTP(repo, newTestOTPEmail(), "123456"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode, got: %v", err)
		}
	})

	t.Run("expired code is rejected", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")
		execTest(t, pool, "UPDATE otp_requests SET expires_at = NOW() - INTERVAL '1 second' WHERE email = $1", email)

		if err := verifyOTP(repo, email, "123456"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode, got: %v", err)
		}
	})

	t.Run("ttl sets the expiry", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		var seconds int64
		err := pool.QueryRow(context.Background(),
			"SELECT EXTRACT(EPOCH FROM expires_at - created_at)::bigint FROM otp_requests WHERE email = $1",
			email).Scan(&seconds)
		if err != nil {
			t.Fatal(err)
		}
		if seconds != 600 {
			t.Errorf("expected 600s until expiry, got %d", seconds)
		}
	})

	t.Run("five wrong guesses kill the code", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		for range 5 {
			_ = verifyOTP(repo, email, "000000")
		}
		if err := verifyOTP(repo, email, "123456"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected the right code to fail after 5 wrong guesses, got: %v", err)
		}
	})

	t.Run("a new code replaces the old one", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "111111")
		mustCreateOTP(t, repo, email, "222222")

		if err := verifyOTP(repo, email, "111111"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected the old code to fail, got: %v", err)
		}
		if err := verifyOTP(repo, email, "222222"); err != nil {
			t.Fatalf("expected the new code to verify, got: %v", err)
		}
	})

	t.Run("a new code resets the attempts", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "111111")
		for range 5 {
			_ = verifyOTP(repo, email, "000000")
		}
		mustCreateOTP(t, repo, email, "222222")

		if err := verifyOTP(repo, email, "222222"); err != nil {
			t.Fatalf("expected the new code to verify, got: %v", err)
		}
	})

	t.Run("an old code stays dead after the new one is used", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "111111")
		mustCreateOTP(t, repo, email, "222222")

		if err := verifyOTP(repo, email, "222222"); err != nil {
			t.Fatal(err)
		}
		if err := verifyOTP(repo, email, "111111"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected the old code to stay invalid, got: %v", err)
		}
	})

	t.Run("parallel guesses never exceed max_attempts", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		var wg sync.WaitGroup
		for range 20 {
			wg.Go(func() { _ = verifyOTP(repo, email, "000000") })
		}
		wg.Wait()

		var attempts int
		err := pool.QueryRow(context.Background(),
			"SELECT attempts_count FROM otp_requests WHERE email = $1", email).Scan(&attempts)
		if err != nil {
			t.Fatal(err)
		}
		if attempts != 5 {
			t.Errorf("expected attempts_count to stop at 5, got %d", attempts)
		}
	})

	t.Run("parallel correct guesses log in exactly once", func(t *testing.T) {
		email := newTestOTPEmail()
		mustCreateOTP(t, repo, email, "123456")

		var wg sync.WaitGroup
		var ok atomic.Int32
		for range 10 {
			wg.Go(func() {
				if verifyOTP(repo, email, "123456") == nil {
					ok.Add(1)
				}
			})
		}
		wg.Wait()

		if ok.Load() != 1 {
			t.Errorf("expected exactly one successful verify, got %d", ok.Load())
		}
	})
}
