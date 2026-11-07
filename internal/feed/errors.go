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

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrForbidden, Status: http.StatusForbidden, Code: "forbidden"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrInvalidUniversity, Status: http.StatusBadRequest, Code: "invalid_university"},
	{Err: ErrInvalidMedia, Status: http.StatusBadRequest, Code: "invalid_media"},
	{Err: ErrKeyReused, Status: http.StatusUnprocessableEntity, Code: "idempotency_key_reused"},
	{Err: ErrResourceDeleted, Status: http.StatusGone, Code: "resource_deleted"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
