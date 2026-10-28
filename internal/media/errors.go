package media

import "errors"

var (
	ErrInvalidUpload  = errors.New("invalid media upload")
	ErrNotFound       = errors.New("media not found")
	ErrUploadMismatch = errors.New("uploaded media is missing or does not match")
)
