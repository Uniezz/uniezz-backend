package study

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

type professorReviewChange struct {
	ProfessorID uuid.UUID
	Content     string
}

type professorReviewQuery struct {
	ViewerID    uuid.UUID
	ProfessorID uuid.UUID
	Cursor      string
	Limit       int
}

func (q professorReviewQuery) filter() (professorReviewFilter, error) {
	after, err := decodePageCursor(q.Cursor)
	if err != nil {
		return professorReviewFilter{}, err
	}
	return professorReviewFilter{ViewerID: q.ViewerID, ProfessorID: q.ProfessorID, After: after, Limit: q.Limit + 1}, nil
}

func (m Module) reviewProfessor(ctx context.Context, userID uuid.UUID, change professorReviewChange) (*ProfessorReview, error) {
	content, err := cleanComment(change.Content)
	if err != nil {
		text.ReportNotAllowed(ctx, userID, err)
		return nil, err
	}

	var row *professorReviewRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.allowCreate(ctx, userID); err != nil {
			return err
		}
		if err := m.professors.lockActive(ctx, change.ProfessorID); err != nil {
			return err
		}
		row, err = m.reviews.save(ctx, change.ProfessorID, userID, content)
		return err
	})
	if err != nil {
		return nil, err
	}
	review := toProfessorReview(*row, userID)
	return &review, nil
}

func (m Module) listProfessorReviews(ctx context.Context, q professorReviewQuery) ([]ProfessorReview, *string, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, nil, err
	}
	if _, err := m.professors.get(ctx, q.ViewerID, q.ProfessorID); err != nil {
		return nil, nil, err
	}
	rows, err := m.reviews.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last professorReviewRow) any {
		return pageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	reviews := make([]ProfessorReview, 0, len(rows))
	for _, row := range rows {
		reviews = append(reviews, toProfessorReview(row, q.ViewerID))
	}
	return reviews, next, nil
}

func (m Module) deleteProfessorReview(ctx context.Context, userID, professorID uuid.UUID) error {
	return m.reviews.delete(ctx, professorID, userID)
}

func toProfessorReview(row professorReviewRow, viewerID uuid.UUID) ProfessorReview {
	return ProfessorReview{
		ID:          row.ID,
		ProfessorID: row.ProfessorID,
		Content:     row.Content,
		IsMine:      row.AuthorID == viewerID,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
