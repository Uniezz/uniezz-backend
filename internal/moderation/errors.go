package moderation

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound      = errors.New("reported content not found")
	ErrInvalidInput  = errors.New("invalid input")
	ErrSelfReport    = errors.New("users cannot report themselves or their own content")
	ErrAlreadyReport = errors.New("already reported")
	ErrRateLimited   = errors.New("too many reports")
)

var errorResponses = []struct {
	err    error
	status int
	code   string
}{
	{ErrNotFound, http.StatusNotFound, "not_found"},
	{ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
	{ErrSelfReport, http.StatusBadRequest, "self_report"},
	{ErrAlreadyReport, http.StatusConflict, "already_reported"},
	{ErrRateLimited, http.StatusTooManyRequests, "rate_limited"},
}

func apiError(err error) error {
	for _, e := range errorResponses {
		if errors.Is(err, e.err) {
			return httpapi.Error(e.status, e.code, err)
		}
	}
	return httpapi.Error(http.StatusInternalServerError, "internal_error", err)
}

func unauthorized() error {
	return httpapi.Error(http.StatusUnauthorized, "unauthorized", nil)
}
