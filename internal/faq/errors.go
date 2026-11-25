package faq

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrContentNotAllowed = errors.New("content not allowed")
	ErrInvalidInput      = errors.New("invalid input")
	ErrForbidden         = errors.New("only a configured publisher may publish or approve")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrNotFound, Status: http.StatusNotFound, Code: "not_found"},
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrContentNotAllowed, Status: http.StatusBadRequest, Code: "content_not_allowed"},
	{Err: ErrForbidden, Status: http.StatusForbidden, Code: "forbidden"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
