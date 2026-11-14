package study

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type professorHandler struct {
	study Module
}

type Professor struct {
	ID            uuid.UUID `json:"id" format:"uuid"`
	Name          string    `json:"name" example:"Anna Maria Nowak"`
	Degree        *string   `json:"degree" nullable:"true" doc:"Degree as POL-on lists it, null when POL-on has none" example:"dr hab."`
	UniversityID  string    `json:"universityId" example:"umcs"`
	RatingAverage float64   `json:"ratingAverage" doc:"Average student rating from 1 to 5, 0 when nobody rated yet" example:"4.5"`
	RatingCount   int       `json:"ratingCount" example:"12"`
	MyRating      *int      `json:"myRating" nullable:"true" minimum:"1" maximum:"5" doc:"The signed-in student's rating, null when they have not rated the professor" example:"5"`
}

type listProfessorsInput struct {
	httpapi.PageQuery
	UniversityID string `query:"universityId" maxLength:"32" example:"umcs"`
	Search       string `query:"q" maxLength:"200" doc:"Up to 5 words of at least 3 characters, matched against the professor's name" example:"nowak"`
}

type listProfessorsOutput struct {
	Body struct {
		Items      []Professor `json:"items"`
		NextCursor *string     `json:"nextCursor" nullable:"true" doc:"Null on the last page"`
	}
}

var listProfessorsOperation = auth.Protected(huma.Operation{
	OperationID: "listProfessors",
	Method:      http.MethodGet,
	Path:        "/study/professors",
	Summary:     "List professors",
	Description: "Current staff of the universities Uniezz serves, as listed in POL-on, ordered by name. The list is refreshed monthly; ratings stay.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *professorHandler) listProfessors(ctx context.Context, in *listProfessorsInput) (*listProfessorsOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.study.listProfessors(ctx, professorQuery{
		ViewerID:     viewerID,
		UniversityID: in.UniversityID,
		Search:       in.Search,
		Cursor:       in.Cursor,
		Limit:        in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listProfessorsOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type getProfessorInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getProfessorOutput struct {
	Body Professor
}

var getProfessorOperation = auth.Protected(huma.Operation{
	OperationID: "getProfessor",
	Method:      http.MethodGet,
	Path:        "/study/professors/{id}",
	Summary:     "Get a professor",
	Description: "A professor who left the POL-on list is not found.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusNotFound},
})

func (h *professorHandler) getProfessor(ctx context.Context, in *getProfessorInput) (*getProfessorOutput, error) {
	viewerID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	professor, err := h.study.getProfessor(ctx, viewerID, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getProfessorOutput{Body: *professor}, nil
}

type rateProfessorInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Rating int `json:"rating" minimum:"1" maximum:"5" example:"5"`
	}
}

type rateProfessorOutput struct {
	Body Professor
}

var rateProfessorOperation = auth.Protected(huma.Operation{
	OperationID: "rateProfessor",
	Method:      http.MethodPut,
	Path:        "/study/professors/{id}/rating",
	Summary:     "Rate a professor",
	Description: "One rating per student; rating again replaces the previous one. A professor who left the POL-on list cannot be rated.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *professorHandler) rateProfessor(ctx context.Context, in *rateProfessorInput) (*rateProfessorOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	professor, err := h.study.rateProfessor(ctx, userID, in.ID, in.Body.Rating)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &rateProfessorOutput{Body: *professor}, nil
}
