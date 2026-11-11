package study

import "github.com/Uniezz/uniezz-backend/internal/httpapi"

func decodePageCursor(cursor string) (*pageCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	var after pageCursor
	if err := httpapi.DecodeCursor(cursor, &after); err != nil {
		return nil, err
	}
	return &after, nil
}

func decodeNameCursor(cursor string) (*nameCursor, error) {
	if cursor == "" {
		return nil, nil
	}
	var after nameCursor
	if err := httpapi.DecodeCursor(cursor, &after); err != nil {
		return nil, err
	}
	return &after, nil
}
