package auth

import (
	"errors"
	"net/http"
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

func httpStatus(err error) int {
	switch {
	case errors.Is(err, ErrUnknownUniversity),
		errors.Is(err, ErrInvalidIdentity),
		errors.Is(err, ErrWrongAuthMethod),
		errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrStateNotFound):
		return http.StatusBadRequest
	case errors.Is(err, ErrInvalidCode),
		errors.Is(err, ErrInvalidTokenString),
		errors.Is(err, ErrSessionNotFound):
		return http.StatusUnauthorized
	case errors.Is(err, ErrNotStudent):
		return http.StatusForbidden
	case errors.Is(err, ErrRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrUpstream):
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}
