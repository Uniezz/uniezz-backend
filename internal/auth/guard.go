package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type principalKey struct{}

type principal struct {
	userID    uuid.UUID
	sessionID uuid.UUID
}

func UserIDFrom(ctx context.Context) (uuid.UUID, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p.userID, ok
}

type guard struct {
	sessions   sessionStore
	sessionTTL time.Duration
	now        func() time.Time
}

var _ sessionStore = (*SessionRepository)(nil)

type sessionStore interface {
	GetActiveSessionByHash(ctx context.Context, tokenHash []byte) (*Session, error)
	ExtendSession(ctx context.Context, id uuid.UUID, expiresAt time.Time) error
}

func (m Module) Guard(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if !requiresBearer(ctx.Operation()) {
			next(ctx)
			return
		}

		p, err := m.guard.authenticate(ctx.Context(), ctx.Header("Authorization"))
		if err != nil {
			status, _ := errorResponse(err)
			_ = huma.WriteErr(api, ctx, status, "authenticate session", err)
			return
		}
		next(huma.WithValue(ctx, principalKey{}, p))
	}
}

func requiresBearer(op *huma.Operation) bool {
	return op != nil && slices.ContainsFunc(op.Security, func(req map[string][]string) bool {
		_, ok := req[httpapi.BearerAuth]
		return ok
	})
}

func (g *guard) authenticate(ctx context.Context, header string) (principal, error) {
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return principal{}, ErrInvalidTokenString
	}
	hash, err := HashTokenString(token)
	if err != nil {
		return principal{}, fmt.Errorf("%w: %w", ErrInvalidTokenString, err)
	}

	s, err := g.sessions.GetActiveSessionByHash(ctx, hash[:])
	if err != nil {
		return principal{}, err
	}

	now := g.now()
	if s.ExpiresAt.Sub(now) < g.sessionTTL/2 {
		if err := g.sessions.ExtendSession(ctx, s.ID, now.Add(g.sessionTTL)); err != nil && !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "extend session", slog.Any("error", err))
		}
	}

	return principal{userID: s.UserID, sessionID: s.ID}, nil
}

var bearerSecurity = []map[string][]string{{httpapi.BearerAuth: {}}}

func Protected(op huma.Operation) huma.Operation {
	op.Security = bearerSecurity
	if !slices.Contains(op.Errors, http.StatusUnauthorized) {
		op.Errors = append(slices.Clone(op.Errors), http.StatusUnauthorized)
	}
	return op
}
