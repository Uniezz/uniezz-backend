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

var errorResponses = httpapi.ErrorResponses{
	{Err: ErrInvalidUpload, Status: http.StatusBadRequest, Code: "invalid_upload"},
}
