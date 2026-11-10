package faq

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var ErrInvalidInput = errors.New("invalid input")

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrInvalidInput, Status: http.StatusBadRequest, Code: "invalid_input"},
	{Err: httpapi.ErrInvalidCursor, Status: http.StatusBadRequest, Code: "invalid_cursor"},
}
