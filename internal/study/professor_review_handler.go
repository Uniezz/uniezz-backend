package study

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type professorReviewHandler struct {
	study Module
}

type ProfessorReview struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	ProfessorID uuid.UUID `json:"professorId" format:"uuid"`
	Content     string    `json:"content" example:"Oral exam, three questions from the published list."`
	IsMine      bool      `json:"isMine" doc:"True when the signed-in student wrote the review and may change or delete it"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type reviewProfessorInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Content string `json:"content" minLength:"1" maxLength:"4000" doc:"Up to 1000 characters after HTML is stripped" example:"Oral exam, three questions from the published list."`
	}
}

type reviewProfessorOutput struct {
	Body ProfessorReview
}

var reviewProfessorOperation = auth.Authenticated(huma.Operation{
	OperationID: "reviewProfessor",
	Method:      http.MethodPut,
	Path:        "/study/professors/{id}/review",
	Summary:     "Review a professor",
	Description: "One review per student; reviewing again replaces the text. Reviews are shown without their author. Counts toward the limit of 30 creates an hour.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusTooManyRequests},
})

func (h *professorReviewHandler) reviewProfessor(ctx context.Context, in *reviewProfessorInput) (*reviewProfessorOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	review, err := h.study.reviewProfessor(ctx, userID, professorReviewChange{ProfessorID: in.ID, Content: in.Body.Content})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &reviewProfessorOutput{Body: *review}, nil
}

type listProfessorReviewsInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
	httpapi.PageQuery
}

type listProfessorReviewsOutput struct {
	Body struct {
		Items      []ProfessorReview `json:"items"`
		NextCursor *string           `json:"nextCursor" nullable:"true" doc:"Null on the last page"`
	}
}

var listProfessorReviewsOperation = auth.Authenticated(huma.Operation{
	OperationID: "listProfessorReviews",
	Method:      http.MethodGet,
	Path:        "/study/professors/{id}/reviews",
	Summary:     "List the reviews of a professor",
	Description: "Newest first. Reviews of blocked students are left out.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *professorReviewHandler) listProfessorReviews(ctx context.Context, in *listProfessorReviewsInput) (*listProfessorReviewsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.study.listProfessorReviews(ctx, professorReviewQuery{
		ViewerID:    viewerID,
		ProfessorID: in.ID,
		Cursor:      in.Cursor,
		Limit:       in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listProfessorReviewsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type deleteMyProfessorReviewInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteMyProfessorReviewOperation = auth.Authenticated(huma.Operation{
	OperationID:   "deleteMyProfessorReview",
	Method:        http.MethodDelete,
	Path:          "/study/professors/{id}/review",
	Summary:       "Delete the student's own review of a professor",
	Description:   "A review that is already gone is left as it is and the call still succeeds.",
	Tags:          []string{"study"},
	DefaultStatus: http.StatusNoContent,
})

func (h *professorReviewHandler) deleteMyProfessorReview(ctx context.Context, in *deleteMyProfessorReviewInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.study.deleteProfessorReview(ctx, userID, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
