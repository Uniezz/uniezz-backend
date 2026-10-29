package media

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var (
	ErrInvalidUpload  = errors.New("invalid media upload")
	ErrNotFound       = errors.New("media not found")
	ErrUploadMismatch = errors.New("uploaded media is missing or does not match")
)

var errorResponses = []struct {
	err    error
	status int
	code   string
}{
	{ErrInvalidUpload, http.StatusBadRequest, "invalid_upload"},
}

func apiError(err error) error {
	for _, e := range errorResponses {
		if errors.Is(err, e.err) {
			return httpapi.Error(e.status, e.code, err)
		}
	}
	return httpapi.Error(http.StatusInternalServerError, "internal_error", err)
}
