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
	userID     uuid.UUID
	sessionID  uuid.UUID
	signedInAt time.Time
	roles      []Role
}

func UserIDFrom(ctx context.Context) (uuid.UUID, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p.userID, ok
}

func SignedInAt(ctx context.Context) (time.Time, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p.signedInAt, ok
}

func RolesFrom(ctx context.Context) ([]Role, bool) {
	p, ok := ctx.Value(principalKey{}).(principal)
	return p.roles, ok
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
		roles, ok := bearerRoles(ctx.Operation())
		if !ok {
			next(ctx)
			return
		}

		p, err := m.guard.authenticate(ctx.Context(), ctx.Header("Authorization"))
		if err != nil {
			status, _ := errorResponses.Lookup(err)
			_ = huma.WriteErr(api, ctx, status, "authenticate session", err)
			return
		}
		if err := p.authorize(roles); err != nil {
			status, _ := errorResponses.Lookup(err)
			_ = huma.WriteErr(api, ctx, status, "authorize role", err)
			return
		}
		next(huma.WithValue(ctx, principalKey{}, p))
	}
}

func bearerRoles(op *huma.Operation) ([]string, bool) {
	if op == nil {
		return nil, false
	}
	for _, req := range op.Security {
		if roles, ok := req[httpapi.BearerAuth]; ok {
			return roles, true
		}
	}
	return nil, false
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

	return principal{userID: s.UserID, sessionID: s.ID, signedInAt: s.CreatedAt, roles: s.Roles}, nil
}

var bearerSecurity = []map[string][]string{{httpapi.BearerAuth: {}}}

func Authenticated(op huma.Operation) huma.Operation {
	op.Security = bearerSecurity
	if !slices.Contains(op.Errors, http.StatusUnauthorized) {
		op.Errors = append(slices.Clone(op.Errors), http.StatusUnauthorized)
	}
	return op
}

func Authorized(op huma.Operation, roles ...Role) huma.Operation {
	op = Authenticated(op)
	scopes := make([]string, 0, len(roles))
	for _, role := range roles {
		scopes = append(scopes, string(role))
	}
	op.Security = []map[string][]string{{httpapi.BearerAuth: scopes}}
	if !slices.Contains(op.Errors, http.StatusForbidden) {
		op.Errors = append(slices.Clone(op.Errors), http.StatusForbidden)
	}
	return op
}
