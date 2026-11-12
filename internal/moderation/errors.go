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
	ErrForbidden     = errors.New("moderators only")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrSelfReport, Status: http.StatusBadRequest, Code: "self_report"},
	{Err: ErrAlreadyReport, Status: http.StatusConflict, Code: "already_reported"},
	{Err: ErrRateLimited, Status: http.StatusTooManyRequests, Code: "rate_limited"},
	{Err: ErrForbidden, Status: http.StatusForbidden, Code: "forbidden"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
