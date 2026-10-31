package media

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const uploadURLTTL = 10 * time.Minute

type Upload struct {
	MediaID   uuid.UUID         `json:"mediaId" format:"uuid" doc:"ID to pass to the endpoint that uses the file, e.g. avatarMediaId"`
	UploadURL string            `json:"uploadUrl" doc:"Presigned URL to PUT the file to, with the headers below"`
	ExpiresAt time.Time         `json:"expiresAt" doc:"When the upload URL stops working"`
	Headers   map[string]string `json:"headers" doc:"Headers the PUT must send exactly as given; If-None-Match makes the URL write the file only once"`
}

func (m Module) CreateUpload(ctx context.Context, ownerUserID uuid.UUID, kind Kind, contentType string, size int64) (Upload, error) {
	if err := validateUpload(kind, contentType, size); err != nil {
		return Upload{}, err
	}

	key := objectKey(kind, contentType)
	expiresAt := time.Now().Add(uploadURLTTL)

	uploadURL, err := m.storage.presignPut(ctx, key, contentType, size)
	if err != nil {
		return Upload{}, err
	}

	item, err := m.repo.createPending(ctx, ownerUserID, kind, key, contentType, size)
	if err != nil {
		return Upload{}, err
	}

	return Upload{MediaID: item.ID, UploadURL: uploadURL, ExpiresAt: expiresAt, Headers: uploadHeaders(contentType, size)}, nil
}

func uploadHeaders(contentType string, size int64) map[string]string {
	return map[string]string{
		"Content-Type":   contentType,
		"Content-Length": strconv.FormatInt(size, 10),
		"If-None-Match":  "*",
	}
}

func validateUpload(kind Kind, contentType string, size int64) error {
	rule, ok := kindRules[kind]
	if !ok {
		return fmt.Errorf("%w: unsupported kind %q", ErrInvalidUpload, kind)
	}
	if _, ok := extensions[contentType]; !ok {
		return fmt.Errorf("%w: unsupported content type %q", ErrInvalidUpload, contentType)
	}
	if size <= 0 || size > rule.maxSize {
		return fmt.Errorf("%w: size must be between 1 and %d bytes", ErrInvalidUpload, rule.maxSize)
	}
	return nil
}

func objectKey(kind Kind, contentType string) string {
	return kindRules[kind].prefix + "/" + uuid.NewString() + extensions[contentType]
}
