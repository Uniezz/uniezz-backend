package auth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/mail"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
)

const (
	otpCodeTTL    = 10 * time.Minute
	otpSendLimit  = 3
	otpSendWindow = 15 * time.Minute
)

var _ otpSender = (*mail.Sender)(nil)

type otpSender interface {
	SendOTP(ctx context.Context, to, code string) error
}

var _ otpStore = (*otpRequestRepository)(nil)

type otpStore interface {
	create(ctx context.Context, req otpRequest, ttl time.Duration) error
	verify(ctx context.Context, universityID UniversityID, email string, codeHash []byte) error
}

var _ sendLimiter = (*ratelimit.Limiter)(nil)

type sendLimiter interface {
	Allow(ctx context.Context, key string) error
}

type otpLogin struct {
	store      otpStore
	sender     otpSender
	limiter    sendLimiter
	hasher     otpHasher
	testEmails []string
}

func newOTPLogin(store otpStore, sender otpSender, limiter sendLimiter, hasher otpHasher, testEmails []string) *otpLogin {
	return &otpLogin{store: store, sender: sender, limiter: limiter, hasher: hasher, testEmails: testEmails}
}

func (l *otpLogin) request(ctx context.Context, universityID UniversityID, email string) error {
	un, err := requireUniversity(universityID, AuthTypeOTP)
	if err != nil {
		return err
	}

	email = normalizeEmail(email)
	if !un.acceptsEmail(email) && !slices.Contains(l.testEmails, email) {
		return fmt.Errorf("%w: %s does not accept this email domain", ErrEmailNotAllowed, un.ID)
	}

	if err := l.limiter.Allow(ctx, string(un.ID)+":"+email); err != nil {
		if errors.Is(err, ratelimit.ErrLimited) {
			return fmt.Errorf("%w: otp send: %w", ErrRateLimited, err)
		}
		return err
	}

	code := generateOTPCode()
	req := otpRequest{UniversityID: un.ID, Email: email, CodeHash: l.hasher.hash(code)}
	if err := l.store.create(ctx, req, otpCodeTTL); err != nil {
		return err
	}

	if err := l.sender.SendOTP(ctx, email, code); err != nil {
		return fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	return nil
}

func (l *otpLogin) verify(ctx context.Context, universityID UniversityID, email, code string) (Identity, error) {
	un, err := requireUniversity(universityID, AuthTypeOTP)
	if err != nil {
		return Identity{}, err
	}

	email = normalizeEmail(email)
	if err := l.store.verify(ctx, un.ID, email, l.hasher.hash(code)); err != nil {
		return Identity{}, err
	}

	return Identity{UniversityID: un.ID, Email: email}, nil
}
