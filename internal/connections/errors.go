package connections

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound            = errors.New("not found")
	ErrInvalidInput        = errors.New("invalid input")
	ErrDatingRequiresAdult = errors.New("dating requires a birth date at least 18 years ago")
	ErrRateLimited         = errors.New("too many likes")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrDatingRequiresAdult, Status: http.StatusBadRequest, Code: "dating_requires_adult"},
	{Err: ErrRateLimited, Status: http.StatusTooManyRequests, Code: "rate_limited"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
