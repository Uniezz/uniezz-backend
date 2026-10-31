package feed

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("only the author may change this")
	ErrInvalidInput      = errors.New("invalid input")
	ErrInvalidUniversity = errors.New("posts may target only the author's university or all")
	ErrInvalidMedia      = errors.New("media is missing, not owned by the user or not uploaded")
	ErrKeyReused         = errors.New("idempotency key was used with a different request")
	ErrResourceDeleted   = errors.New("the resource created with this idempotency key was deleted")
)

var errorResponses = []struct {
	err    error
	status int
	code   string
}{
	{ErrNotFound, http.StatusNotFound, "not_found"},
	{ErrForbidden, http.StatusForbidden, "forbidden"},
	{ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
	{ErrInvalidUniversity, http.StatusBadRequest, "invalid_university"},
	{ErrInvalidMedia, http.StatusBadRequest, "invalid_media"},
	{ErrKeyReused, http.StatusUnprocessableEntity, "idempotency_key_reused"},
	{ErrResourceDeleted, http.StatusGone, "resource_deleted"},
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
