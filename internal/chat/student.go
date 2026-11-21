package chat

import (
	"context"
	"log/slog"

	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/google/uuid"
)

func (m Module) studentMap(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Student, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]Student{}, nil
	}
	rows, err := m.students.getMany(ctx, ids)
	if err != nil {
		return nil, err
	}

	students := make(map[uuid.UUID]Student, len(rows))
	for _, row := range rows {
		id := row.ID
		university := row.UniversityID
		students[id] = Student{
			ID:           &id,
			FirstName:    row.FirstName,
			LastName:     row.LastName,
			UniversityID: &university,
			AvatarURL:    m.mediaURL(ctx, row.Avatar),
		}
	}
	return students, nil
}

func studentOf(students map[uuid.UUID]Student, id *uuid.UUID) Student {
	if id == nil {
		return Student{Deleted: true}
	}
	if s, ok := students[*id]; ok {
		return s
	}
	return Student{Deleted: true}
}

func (m Module) mediaURL(ctx context.Context, ref *media.Ref) *string {
	if ref == nil {
		return nil
	}
	url, err := m.media.URLOf(ctx, *ref)
	if err != nil {
		slog.WarnContext(ctx, "build media url", slog.Any("error", err))
		return nil
	}
	return &url
}
