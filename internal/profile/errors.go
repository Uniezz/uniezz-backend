package profile

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrGenderImmutable = errors.New("gender cannot be changed or cleared once set")
	ErrNotFound        = errors.New("not found")
	ErrInvalidInput    = errors.New("invalid input")
	ErrInvalidAvatar   = errors.New("avatar media is missing, not owned by the user or not uploaded")
	ErrInvalidBanner   = errors.New("banner media is missing, not owned by the user or not uploaded")
	ErrReauthRequired  = errors.New("sign in again to delete the account")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrGenderImmutable, Status: http.StatusBadRequest, Code: "gender_immutable"},
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrInvalidAvatar, Status: http.StatusBadRequest, Code: "invalid_avatar"},
	{Err: ErrInvalidBanner, Status: http.StatusBadRequest, Code: "invalid_banner"},
	{Err: ErrReauthRequired, Status: http.StatusForbidden, Code: "reauthentication_required"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
