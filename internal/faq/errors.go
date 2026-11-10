package faq

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrForbidden    = errors.New("only the staff address may publish")
)

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: ErrForbidden, Status: http.StatusForbidden, Code: "forbidden"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
