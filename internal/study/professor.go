package study

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

type professorQuery struct {
	ViewerID     uuid.UUID
	UniversityID string
	Search       string
	Cursor       string
	Limit        int
}

func (q professorQuery) filter() (professorFilter, error) {
	if q.UniversityID != "" {
		if _, err := auth.GetUniversityByID(auth.UniversityID(q.UniversityID)); err != nil {
			return professorFilter{}, ErrInvalidInput
		}
	}
	words, err := text.SearchWords(q.Search)
	if err != nil {
		return professorFilter{}, ErrInvalidInput
	}
	after, err := decodeNameCursor(q.Cursor)
	if err != nil {
		return professorFilter{}, err
	}
	return professorFilter{ViewerID: q.ViewerID, UniversityID: q.UniversityID, Words: words, After: after, Limit: q.Limit + 1}, nil
}

func (m Module) listProfessors(ctx context.Context, q professorQuery) ([]Professor, *string, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, nil, err
	}
	rows, err := m.professors.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last professorRow) any {
		return nameCursor{Name: last.Name, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	professors := make([]Professor, 0, len(rows))
	for _, row := range rows {
		professors = append(professors, toProfessor(row))
	}
	return professors, next, nil
}

func (m Module) getProfessor(ctx context.Context, viewerID, id uuid.UUID) (*Professor, error) {
	row, err := m.professors.get(ctx, viewerID, id)
	if err != nil {
		return nil, err
	}
	professor := toProfessor(*row)
	return &professor, nil
}

func (m Module) rateProfessor(ctx context.Context, userID, id uuid.UUID, rating int) (*Professor, error) {
	if !validRating(rating) {
		return nil, ErrInvalidInput
	}

	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.professors.lockActive(ctx, id); err != nil {
			return err
		}
		old, found, err := m.professors.getRating(ctx, id, userID)
		if err != nil {
			return err
		}
		if err := m.professors.saveRating(ctx, id, userID, rating); err != nil {
			return err
		}
		added := 0
		if !found {
			added = 1
		}
		return m.professors.addToRating(ctx, id, added, rating-old)
	})
	if err != nil {
		return nil, err
	}
	return m.getProfessor(ctx, userID, id)
}

func (m Module) removeProfessorRating(ctx context.Context, userID, id uuid.UUID) error {
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.professors.lock(ctx, id); err != nil {
			return err
		}
		old, found, err := m.professors.deleteRating(ctx, id, userID)
		if err != nil || !found {
			return err
		}
		return m.professors.addToRating(ctx, id, -1, -old)
	})
}

func toProfessor(row professorRow) Professor {
	return Professor{
		ID:            row.ID,
		Name:          row.Name,
		Degree:        row.Degree,
		UniversityID:  row.UniversityID,
		RatingAverage: ratingAverage(row.RatingSum, row.RatingCount),
		RatingCount:   row.RatingCount,
		MyRating:      row.MyRating,
	}
}
