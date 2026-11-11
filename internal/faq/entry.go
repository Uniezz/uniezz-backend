package faq

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

const (
	statusProposed  = "proposed"
	statusPublished = "published"
)

type Entry struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Status    string    `json:"status" enum:"proposed,published,rejected"`
	CreatedAt time.Time `json:"createdAt"`
}

type pageCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

func (m Module) listPublished(ctx context.Context, cursor string, limit int) ([]Entry, *string, error) {
	return m.list(ctx, listQuery{Status: statusPublished, Cursor: cursor, Limit: limit})
}

func (m Module) listMine(ctx context.Context, authorID uuid.UUID, cursor string, limit int) ([]Entry, *string, error) {
	return m.list(ctx, listQuery{AuthorID: &authorID, Cursor: cursor, Limit: limit})
}

func (m Module) propose(ctx context.Context, authorID uuid.UUID, question, answer string) (*Entry, error) {
	return m.create(ctx, authorID, question, answer, statusProposed)
}

func (m Module) publish(ctx context.Context, authorID uuid.UUID, question, answer string) (*Entry, error) {
	email, err := m.entries.userEmail(ctx, authorID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrForbidden
	}
	if err != nil {
		return nil, err
	}
	if !m.publishers[strings.ToLower(email)] {
		return nil, ErrForbidden
	}
	return m.create(ctx, authorID, question, answer, statusPublished)
}

func (m Module) create(ctx context.Context, authorID uuid.UUID, question, answer, status string) (*Entry, error) {
	question, err := cleanText(question, false, minQuestionLength, maxQuestionLength)
	if err != nil {
		return nil, err
	}
	answer, err = cleanText(answer, true, minAnswerLength, maxAnswerLength)
	if err != nil {
		return nil, err
	}
	row, err := m.entries.insert(ctx, authorID, question, answer, status)
	if err != nil {
		return nil, err
	}
	entry := toEntry(*row)
	return &entry, nil
}

type listQuery struct {
	AuthorID *uuid.UUID
	Status   string
	Cursor   string
	Limit    int
}

func (m Module) list(ctx context.Context, q listQuery) ([]Entry, *string, error) {
	var after *pageCursor
	if q.Cursor != "" {
		after = &pageCursor{}
		if err := httpapi.DecodeCursor(q.Cursor, after); err != nil {
			return nil, nil, err
		}
	}
	rows, err := m.entries.list(ctx, q.AuthorID, q.Status, after, q.Limit+1)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last entryRow) any {
		return pageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	entries := make([]Entry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, toEntry(row))
	}
	return entries, next, nil
}

func toEntry(row entryRow) Entry {
	return Entry(row)
}
