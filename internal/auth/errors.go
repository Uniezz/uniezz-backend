package auth

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrUnknownUniversity = errors.New("unknown university")
	ErrWrongAuthMethod   = errors.New("university does not support this authentication method")
	ErrInvalidInput      = errors.New("invalid input")
	ErrEmailNotAllowed   = errors.New("email domain not allowed for this university")
	ErrStateNotFound     = errors.New("login state not found or expired")
	ErrInvalidCode       = errors.New("invalid or expired code")
	ErrNotStudent        = errors.New("not an active student")
	ErrRateLimited       = errors.New("too many attempts")
	ErrUpstream          = errors.New("identity provider unavailable")

	ErrInvalidIdentity = errors.New("invalid identity")

	ErrInvalidTokenString = errors.New("invalid token string")

	ErrSessionNotFound = errors.New("session not found")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrInvalidIdentity, Status: http.StatusInternalServerError, Code: "internal_error"},
	{Err: ErrUnknownUniversity, Status: http.StatusBadRequest, Code: "unknown_university"},
	{Err: ErrWrongAuthMethod, Status: http.StatusBadRequest, Code: "wrong_auth_method"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrEmailNotAllowed, Status: http.StatusBadRequest, Code: "email_not_allowed"},
	{Err: ErrStateNotFound, Status: http.StatusBadRequest, Code: "login_expired"},
	{Err: ErrInvalidCode, Status: http.StatusUnauthorized, Code: "invalid_code"},
	{Err: ErrInvalidTokenString, Status: http.StatusUnauthorized, Code: "unauthorized"},
	{Err: ErrSessionNotFound, Status: http.StatusUnauthorized, Code: "unauthorized"},
	{Err: ErrNotStudent, Status: http.StatusForbidden, Code: "not_student"},
	{Err: ErrRateLimited, Status: http.StatusTooManyRequests, Code: "rate_limited"},
	{Err: ErrUpstream, Status: http.StatusBadGateway, Code: "provider_unavailable"},
}
