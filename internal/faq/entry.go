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

type EntryText struct {
	Question string `json:"question" minLength:"1" maxLength:"1000" doc:"3 to 200 characters after HTML is stripped"`
	Answer   string `json:"answer" minLength:"1" maxLength:"8000" doc:"1 to 2000 characters after HTML is stripped"`
}

type Entry struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	Language  string    `json:"language" enum:"en,pl,uk,ru,be,de,es" doc:"Language of question and answer"`
	Status    string    `json:"status" enum:"proposed,published,rejected"`
	CreatedAt time.Time `json:"createdAt"`
}

type pageCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

type entryQuery struct {
	ViewerID uuid.UUID
	// Language the viewer reads in; empty means their profile language.
	Language string
	Cursor   string
	Limit    int
}

func (m Module) listPublished(ctx context.Context, q entryQuery) ([]Entry, *string, error) {
	lang, err := m.readingLanguage(ctx, q.ViewerID, q.Language)
	if err != nil {
		return nil, nil, err
	}
	return m.list(ctx, listQuery{Language: lang, Status: statusPublished, Cursor: q.Cursor, Limit: q.Limit})
}

func (m Module) listMine(ctx context.Context, authorID uuid.UUID, cursor string, limit int) ([]Entry, *string, error) {
	lang, err := m.readingLanguage(ctx, authorID, "")
	if err != nil {
		return nil, nil, err
	}
	return m.list(ctx, listQuery{Language: lang, AuthorID: &authorID, Cursor: cursor, Limit: limit})
}

func (m Module) readingLanguage(ctx context.Context, viewerID uuid.UUID, lang string) (string, error) {
	if lang != "" {
		if !isLanguage(lang) {
			return "", ErrInvalidInput
		}
		return lang, nil
	}
	return m.entries.userLanguage(ctx, viewerID)
}

type proposalChange struct {
	AuthorID uuid.UUID
	Language string
	Text     EntryText
}

func (m Module) propose(ctx context.Context, c proposalChange) (*Entry, error) {
	clean, err := cleanEntryText(c.Text)
	if err != nil {
		return nil, err
	}
	if c.Language != "" && !isLanguage(c.Language) {
		return nil, ErrInvalidInput
	}
	lang := c.Language
	if lang == "" {
		if lang, err = m.entries.userLanguage(ctx, c.AuthorID); err != nil {
			return nil, err
		}
	}
	row, err := m.entries.insert(ctx, entryFields{
		AuthorID: c.AuthorID,
		Language: lang,
		Texts:    map[string]EntryText{lang: clean},
		Status:   statusProposed,
	})
	if err != nil {
		return nil, err
	}
	entry := toEntry(*row)
	return &entry, nil
}

func (m Module) publish(ctx context.Context, authorID uuid.UUID, texts map[string]EntryText) (*Entry, error) {
	clean, err := cleanAllTexts(texts)
	if err != nil {
		return nil, err
	}
	if err := m.checkPublisher(ctx, authorID); err != nil {
		return nil, err
	}
	row, err := m.entries.insert(ctx, entryFields{
		AuthorID: authorID,
		Language: languages[0],
		Texts:    clean,
		Status:   statusPublished,
	})
	if err != nil {
		return nil, err
	}
	entry := toEntry(*row)
	return &entry, nil
}

func (m Module) checkPublisher(ctx context.Context, userID uuid.UUID) error {
	email, err := m.entries.userEmail(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if !m.publishers[strings.ToLower(email)] {
		return ErrForbidden
	}
	return nil
}

type listQuery struct {
	// Language picks the text in that language; empty gives the author's original.
	Language string
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
	rows, err := m.entries.list(ctx, entryFilter{
		Language: q.Language,
		AuthorID: q.AuthorID,
		Status:   q.Status,
		After:    after,
		Limit:    q.Limit + 1,
	})
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
