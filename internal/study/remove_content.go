package study

import (
	"context"

	"github.com/google/uuid"
)

func (m Module) RemoveFile(ctx context.Context, id uuid.UUID) error {
	return m.deleteFile(ctx, uuid.Nil, id, true)
}

func (m Module) RemoveCourse(ctx context.Context, id uuid.UUID) error {
	return m.deleteCourse(ctx, uuid.Nil, id, true)
}
