package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

var ErrInvalidCursor = errors.New("invalid cursor")

type PageQuery struct {
	Cursor string `query:"cursor" maxLength:"512" doc:"nextCursor from the previous page; omit for the first page"`
	Limit  int    `query:"limit" minimum:"1" maximum:"50" default:"20" doc:"Maximum number of items in the page"`
}

func EncodeCursor(position any) (string, error) {
	raw, err := json.Marshal(position)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func DecodeCursor(cursor string, position any) error {
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return ErrInvalidCursor
	}
	if err := json.Unmarshal(raw, position); err != nil {
		return ErrInvalidCursor
	}
	return nil
}
