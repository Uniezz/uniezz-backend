package auth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
)

type fakeOTPStore struct {
	created   []otpRequest
	ttl       time.Duration
	createErr error
}

func (s *fakeOTPStore) create(_ context.Context, req otpRequest, ttl time.Duration) error {
	if s.createErr != nil {
		return s.createErr
	}
	s.created = append(s.created, req)
	s.ttl = ttl
	return nil
}

func (s *fakeOTPStore) verify(_ context.Context, universityID UniversityID, email string, codeHash []byte) error {
	for i := len(s.created) - 1; i >= 0; i-- {
		r := s.created[i]
		if r.UniversityID == universityID && r.Email == email {
			if bytes.Equal(r.CodeHash, codeHash) {
				return nil
			}
			return ErrInvalidCode
		}
	}
	return ErrInvalidCode
}

type fakeOTPSender struct {
	to, code string
	calls    int
	err      error
}

func (s *fakeOTPSender) SendOTP(_ context.Context, to, code string) error {
	s.calls++
	s.to, s.code = to, code
	return s.err
}

type fakeLimiter struct {
	keys []string
	err  error
}

func (l *fakeLimiter) Allow(_ context.Context, key string) error {
	l.keys = append(l.keys, key)
	return l.err
}

type otpLoginFixture struct {
	login   *otpLogin
	store   *fakeOTPStore
	sender  *fakeOTPSender
	limiter *fakeLimiter
}

func newOTPLoginFixture() otpLoginFixture {
	f := otpLoginFixture{store: &fakeOTPStore{}, sender: &fakeOTPSender{}, limiter: &fakeLimiter{}}
	f.login = newOTPLogin(f.store, f.sender, f.limiter, newOTPHasher([]byte("test-key")))
	return f
}

func TestOTPLogin_Request(t *testing.T) {
	ctx := context.Background()

	t.Run("stores the hash of the code it sends", func(t *testing.T) {
		f := newOTPLoginFixture()

		if err := f.login.request(ctx, UniversityKUL, " Jan@Student.KUL.pl "); err != nil {
			t.Fatalf("request failed: %v", err)
		}

		if f.sender.to != "jan@student.kul.pl" {
			t.Errorf("expected the code sent to the normalized email, got %q", f.sender.to)
		}
		if len(f.store.created) != 1 {
			t.Fatalf("expected 1 stored request, got %d", len(f.store.created))
		}
		got := f.store.created[0]
		if got.UniversityID != UniversityKUL || got.Email != "jan@student.kul.pl" {
			t.Errorf("unexpected stored request: %+v", got)
		}
		if !bytes.Equal(got.CodeHash, f.login.hasher.hash(f.sender.code)) {
			t.Error("expected the stored hash to be the hash of the sent code")
		}
		if bytes.Contains(got.CodeHash, []byte(f.sender.code)) {
			t.Error("expected the raw code not to be stored")
		}
		if f.store.ttl != 10*time.Minute {
			t.Errorf("expected a 10m ttl, got %v", f.store.ttl)
		}
	})

	t.Run("limits per university and normalized email", func(t *testing.T) {
		f := newOTPLoginFixture()

		if err := f.login.request(ctx, UniversityKUL, "Jan@Student.KUL.pl"); err != nil {
			t.Fatal(err)
		}
		if len(f.limiter.keys) != 1 || f.limiter.keys[0] != "kul:jan@student.kul.pl" {
			t.Errorf("unexpected limiter keys: %q", f.limiter.keys)
		}
	})

	t.Run("rate limited sends nothing", func(t *testing.T) {
		f := newOTPLoginFixture()
		f.limiter.err = ratelimit.ErrLimited

		err := f.login.request(ctx, UniversityKUL, "jan@student.kul.pl")
		if !errors.Is(err, ErrRateLimited) {
			t.Fatalf("expected ErrRateLimited, got: %v", err)
		}
		if len(f.store.created) != 0 || f.sender.calls != 0 {
			t.Error("expected nothing stored or sent")
		}
	})

	t.Run("limiter failure is not a rate limit", func(t *testing.T) {
		f := newOTPLoginFixture()
		f.limiter.err = errors.New("db down")

		err := f.login.request(ctx, UniversityKUL, "jan@student.kul.pl")
		if err == nil || errors.Is(err, ErrRateLimited) {
			t.Fatalf("expected a plain error, got: %v", err)
		}
		if f.sender.calls != 0 {
			t.Error("expected nothing sent")
		}
	})

	t.Run("foreign email domain is rejected before the limiter", func(t *testing.T) {
		f := newOTPLoginFixture()

		err := f.login.request(ctx, UniversityKUL, "jan@gmail.com")
		if !errors.Is(err, ErrEmailNotAllowed) {
			t.Fatalf("expected ErrEmailNotAllowed, got: %v", err)
		}
		if len(f.limiter.keys) != 0 || len(f.store.created) != 0 || f.sender.calls != 0 {
			t.Error("expected no limiter call, nothing stored or sent")
		}
	})

	t.Run("USOS university is rejected", func(t *testing.T) {
		f := newOTPLoginFixture()

		if err := f.login.request(ctx, UniversityUMCS, "jan@umcs.pl"); !errors.Is(err, ErrWrongAuthMethod) {
			t.Fatalf("expected ErrWrongAuthMethod, got: %v", err)
		}
	})

	t.Run("unknown university is rejected", func(t *testing.T) {
		f := newOTPLoginFixture()

		if err := f.login.request(ctx, "nope", "jan@student.kul.pl"); !errors.Is(err, ErrUnknownUniversity) {
			t.Fatalf("expected ErrUnknownUniversity, got: %v", err)
		}
	})

	t.Run("store failure sends nothing", func(t *testing.T) {
		f := newOTPLoginFixture()
		f.store.createErr = errors.New("db down")

		if err := f.login.request(ctx, UniversityKUL, "jan@student.kul.pl"); err == nil {
			t.Fatal("expected an error")
		}
		if f.sender.calls != 0 {
			t.Error("expected nothing sent")
		}
	})

	t.Run("send failure is an upstream error", func(t *testing.T) {
		f := newOTPLoginFixture()
		f.sender.err = errors.New("resend down")

		if err := f.login.request(ctx, UniversityKUL, "jan@student.kul.pl"); !errors.Is(err, ErrUpstream) {
			t.Fatalf("expected ErrUpstream, got: %v", err)
		}
	})
}

func TestOTPLogin_Verify(t *testing.T) {
	ctx := context.Background()

	t.Run("sent code returns the identity", func(t *testing.T) {
		f := newOTPLoginFixture()
		if err := f.login.request(ctx, UniversityKUL, "jan@student.kul.pl"); err != nil {
			t.Fatal(err)
		}

		id, err := f.login.verify(ctx, UniversityKUL, " JAN@student.kul.pl", f.sender.code)
		if err != nil {
			t.Fatalf("verify failed: %v", err)
		}
		want := Identity{UniversityID: UniversityKUL, Email: "jan@student.kul.pl"}
		if id != want {
			t.Errorf("expected %+v, got %+v", want, id)
		}
		if err := id.validate(); err != nil {
			t.Errorf("expected a valid identity, got: %v", err)
		}
	})

	t.Run("wrong code is rejected", func(t *testing.T) {
		f := newOTPLoginFixture()
		if err := f.login.request(ctx, UniversityKUL, "jan@student.kul.pl"); err != nil {
			t.Fatal(err)
		}

		wrong := "000000"
		if f.sender.code == wrong {
			wrong = "111111"
		}
		if _, err := f.login.verify(ctx, UniversityKUL, "jan@student.kul.pl", wrong); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("expected ErrInvalidCode, got: %v", err)
		}
	})

	t.Run("USOS university is rejected", func(t *testing.T) {
		f := newOTPLoginFixture()

		if _, err := f.login.verify(ctx, UniversityUMCS, "jan@umcs.pl", "123456"); !errors.Is(err, ErrWrongAuthMethod) {
			t.Fatalf("expected ErrWrongAuthMethod, got: %v", err)
		}
	})
}
