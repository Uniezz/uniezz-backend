package media

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"

	"github.com/google/uuid"
)

func (m Module) Import(ctx context.Context, ownerUserID uuid.UUID, kind Kind, sourceURL string) (uuid.UUID, error) {
	rule, ok := kindRules[kind]
	if !ok {
		return uuid.Nil, fmt.Errorf("%w: unsupported kind %q", ErrInvalidUpload, kind)
	}
	if u, err := url.Parse(sourceURL); err != nil || u.Scheme != "https" {
		return uuid.Nil, fmt.Errorf("%w: source URL must be https", ErrInvalidUpload)
	}

	contentType, body, err := m.download(ctx, sourceURL, rule.maxSize)
	if err != nil {
		return uuid.Nil, err
	}
	if err := validateUpload(kind, contentType, int64(len(body))); err != nil {
		return uuid.Nil, err
	}

	key := objectKey(kind, contentType)
	item, err := m.repo.createPending(ctx, ownerUserID, kind, key, contentType, int64(len(body)))
	if err != nil {
		return uuid.Nil, err
	}
	if err := m.storage.putObject(ctx, key, contentType, body); err != nil {
		return uuid.Nil, err
	}
	if err := m.repo.markReady(ctx, item.ID, ownerUserID); err != nil {
		return uuid.Nil, err
	}
	return item.ID, nil
}

func (m Module) download(ctx context.Context, sourceURL string, maxSize int64) (string, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %w", ErrInvalidUpload, err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("download media: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("download media: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSize+1))
	if err != nil {
		return "", nil, fmt.Errorf("download media: %w", err)
	}

	contentType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return "", nil, fmt.Errorf("%w: content type: %w", ErrInvalidUpload, err)
	}
	return contentType, body, nil
}
