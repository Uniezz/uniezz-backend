package connections

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound              = errors.New("not found")
	ErrInvalidInput          = errors.New("invalid input")
	ErrDatingRequiresAdult   = errors.New("dating requires a birth date at least 18 years ago")
	ErrRateLimited           = errors.New("too many likes")
	ErrBirthDateImmutable    = errors.New("birth date cannot be changed once set")
	ErrDatingRequiresProfile = errors.New("dating requires a birth date and gender")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrDatingRequiresAdult, Status: http.StatusBadRequest, Code: "dating_requires_adult"},
	{Err: ErrBirthDateImmutable, Status: http.StatusBadRequest, Code: "birth_date_immutable"},
	{Err: ErrRateLimited, Status: http.StatusTooManyRequests, Code: "rate_limited"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
	{Err: ErrDatingRequiresProfile, Status: http.StatusBadRequest, Code: "dating_requires_profile"},
}
