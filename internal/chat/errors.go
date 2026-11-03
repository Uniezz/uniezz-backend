package chat

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("chat thread not found")
)

func apiError(err error) error {
	switch {
	case errors.Is(err, ErrInvalidInput):
		return httpapi.Error(http.StatusBadRequest, "invalid_input", err)
	case errors.Is(err, ErrNotFound):
		return httpapi.Error(http.StatusNotFound, "not_found", err)
	default:
		return httpapi.Error(http.StatusInternalServerError, "internal_error", err)
	}
}
