package feed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
)

const (
	opCreatePost    = "create_post"
	opCreateComment = "create_comment"
)

var errKeyTaken = errors.New("idempotency key taken by a concurrent request")

func fingerprint(input any) ([]byte, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	return sum[:], nil
}

// replayed returns the resource a previous request with the same key created,
// or ErrNotFound when the key is unused.
func (m Module) replayed(ctx context.Context, userID uuid.UUID, operation, key string, fp []byte) (uuid.UUID, error) {
	row, err := m.keys.get(ctx, userID, operation, key)
	if err != nil {
		return uuid.Nil, err
	}
	if !bytes.Equal(row.Fingerprint, fp) {
		return uuid.Nil, ErrKeyReused
	}
	return row.ResourceID, nil
}

func (m Module) claimKey(ctx context.Context, userID uuid.UUID, operation, key string, fp []byte, resourceID uuid.UUID) error {
	if key == "" {
		return nil
	}
	inserted, err := m.keys.insert(ctx, userID, operation, key, fp, resourceID)
	if err != nil {
		return err
	}
	if !inserted {
		return errKeyTaken
	}
	return nil
}

func resourceGone(err error) error {
	if errors.Is(err, ErrNotFound) {
		return ErrResourceDeleted
	}
	return err
}
