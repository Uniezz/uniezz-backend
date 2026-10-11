package auth

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpjson"
)

var (
	ErrUnknownUniversity = errors.New("unknown university")
	ErrWrongAuthMethod   = errors.New("university does not support this authentication method")
	ErrInvalidInput      = errors.New("invalid input")
	ErrStateNotFound     = errors.New("login state not found or expired")
	ErrInvalidCode       = errors.New("invalid or expired code")
	ErrNotStudent        = errors.New("not an active student")
	ErrRateLimited       = errors.New("too many attempts")
	ErrUpstream          = errors.New("identity provider unavailable")

	ErrInvalidIdentity = errors.New("invalid identity")

	ErrInvalidTokenString = errors.New("invalid token string")

	ErrSessionNotFound = errors.New("session not found")
)

var errorResponses = []struct {
	err    error
	status int
	code   string
}{
	{ErrInvalidIdentity, http.StatusInternalServerError, "internal_error"},
	{httpjson.ErrInvalidBody, http.StatusBadRequest, "invalid_input"},
	{ErrUnknownUniversity, http.StatusBadRequest, "unknown_university"},
	{ErrWrongAuthMethod, http.StatusBadRequest, "wrong_auth_method"},
	{ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
	{ErrStateNotFound, http.StatusBadRequest, "login_expired"},
	{ErrInvalidCode, http.StatusUnauthorized, "invalid_code"},
	{ErrInvalidTokenString, http.StatusUnauthorized, "unauthorized"},
	{ErrSessionNotFound, http.StatusUnauthorized, "unauthorized"},
	{ErrNotStudent, http.StatusForbidden, "not_student"},
	{ErrRateLimited, http.StatusTooManyRequests, "rate_limited"},
	{ErrUpstream, http.StatusBadGateway, "provider_unavailable"},
}

func errorResponse(err error) (status int, code string) {
	for _, e := range errorResponses {
		if errors.Is(err, e.err) {
			return e.status, e.code
		}
	}
	return http.StatusInternalServerError, "internal_error"
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := errorResponse(err)
	httpjson.Error(w, r, status, code, err)
}
