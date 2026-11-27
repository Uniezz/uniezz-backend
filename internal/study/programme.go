package study

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

type ProgrammeLevel string

const (
	ProgrammeFirstCycle  ProgrammeLevel = "first_cycle"
	ProgrammeSecondCycle ProgrammeLevel = "second_cycle"
	ProgrammeLongCycle   ProgrammeLevel = "long_cycle"
)

type programmeQuery struct {
	UniversityID string
	FacultyID    uuid.UUID
	Search       string
	Cursor       string
	Limit        int
}

func (q programmeQuery) filter() (programmeFilter, error) {
	words, err := text.SearchWords(q.Search)
	if err != nil {
		return programmeFilter{}, ErrInvalidInput
	}
	after, err := decodeNameCursor(q.Cursor)
	if err != nil {
		return programmeFilter{}, err
	}
	return programmeFilter{
		UniversityID: q.UniversityID,
		FacultyID:    q.FacultyID,
		Words:        words,
		After:        after,
		Limit:        q.Limit + 1,
	}, nil
}

func (m Module) listProgrammes(ctx context.Context, q programmeQuery) ([]CatalogueProgramme, *string, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, nil, err
	}
	rows, err := m.programmes.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last programmeRow) any {
		return nameCursor{Name: last.Name, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	programmes := make([]CatalogueProgramme, 0, len(rows))
	for _, row := range rows {
		programmes = append(programmes, CatalogueProgramme(row))
	}
	return programmes, next, nil
}
