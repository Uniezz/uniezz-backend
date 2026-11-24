package media

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

var reviewedKinds = []Kind{KindAvatar, KindBanner, KindFeed, KindChat}

func (m Module) listUnreviewed(ctx context.Context, cursor string, limit int) ([]Image, *string, error) {
	filter := imageFilter{Kinds: reviewedKinds, Limit: limit + 1}
	if cursor != "" {
		var after imageCursor
		if err := httpapi.DecodeCursor(cursor, &after); err != nil {
			return nil, nil, err
		}
		filter.After = &after
	}

	rows, err := m.repo.listUnreviewed(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, limit, func(last mediaRow) any {
		return imageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}

	images := make([]Image, 0, len(rows))
	for _, row := range rows {
		url, err := m.URLOf(ctx, Ref{Kind: row.Kind, ObjectKey: row.ObjectKey})
		if err != nil {
			return nil, nil, err
		}
		images = append(images, Image{ID: row.ID, Kind: row.Kind, URL: url, OwnerID: row.OwnerUserID, CreatedAt: row.CreatedAt})
	}
	return images, next, nil
}

func (m Module) markReviewed(ctx context.Context, id uuid.UUID) error {
	return m.repo.markReviewed(ctx, id, reviewedKinds)
}

func (m Module) removeImage(ctx context.Context, id uuid.UUID) error {
	objectKey, err := m.repo.markRemoved(ctx, id, reviewedKinds)
	if err != nil {
		return err
	}
	return m.storage.deleteObject(ctx, objectKey)
}
