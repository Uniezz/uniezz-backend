package profile

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound      = errors.New("not found")
	ErrInvalidInput  = errors.New("invalid input")
	ErrInvalidAvatar = errors.New("avatar media is missing, not owned by the user or not uploaded")
)

var errorResponses = []struct {
	err    error
	status int
	code   string
}{
	{ErrNotFound, http.StatusNotFound, "not_found"},
	{ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
	{ErrInvalidAvatar, http.StatusBadRequest, "invalid_avatar"},
	{httpapi.ErrInvalidCursor, http.StatusBadRequest, "invalid_cursor"},
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
