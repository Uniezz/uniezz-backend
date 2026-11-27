package study

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type programmeHandler struct {
	study Module
}

type CatalogueProgramme struct {
	ID           uuid.UUID      `json:"id" format:"uuid"`
	UniversityID string         `json:"universityId" example:"umcs"`
	FacultyID    *uuid.UUID     `json:"facultyId" format:"uuid" nullable:"true" doc:"One of the faculties listed by getUniversities; null when POL-on names none of them"`
	Faculty      *string        `json:"faculty" nullable:"true" doc:"Name of the faculty; null when facultyId is null" example:"Wydział Matematyki, Fizyki i Informatyki"`
	Name         string         `json:"name" example:"Informatyka"`
	Level        ProgrammeLevel `json:"level" enum:"first_cycle,second_cycle,long_cycle" doc:"First-cycle (licencjat, inżynier), second-cycle (magister) or long-cycle (jednolite magisterskie) studies"`
}

type listProgrammesInput struct {
	httpapi.PageQuery
	UniversityID string    `query:"universityId" required:"true" minLength:"1" maxLength:"32" example:"umcs"`
	FacultyID    uuid.UUID `query:"facultyId" format:"uuid" doc:"Only programmes of this faculty; omitted for every faculty"`
	Search       string    `query:"q" maxLength:"200" doc:"Up to 5 words of at least 3 characters, matched against the programme name" example:"informatyka"`
}

type listProgrammesOutput struct {
	Body struct {
		Items      []CatalogueProgramme `json:"items"`
		NextCursor *string              `json:"nextCursor" nullable:"true" doc:"Null on the last page"`
	}
}

var listProgrammesOperation = auth.Authenticated(huma.Operation{
	OperationID: "listProgrammes",
	Method:      http.MethodGet,
	Path:        "/study/programmes",
	Summary:     "List the study programmes of a university",
	Description: "Programmes (kierunki) that the university runs or is closing, as listed in POL-on, ordered by name. The list is refreshed monthly; a programme POL-on no longer lists drops out.",
	Tags:        []string{"study"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *programmeHandler) listProgrammes(ctx context.Context, in *listProgrammesInput) (*listProgrammesOutput, error) {
	if _, ok := auth.UserIDFrom(ctx); !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.study.listProgrammes(ctx, programmeQuery{
		UniversityID: in.UniversityID,
		FacultyID:    in.FacultyID,
		Search:       in.Search,
		Cursor:       in.Cursor,
		Limit:        in.Limit,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listProgrammesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}
