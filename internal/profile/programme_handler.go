package profile

import (
	"context"
	"net/http"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type programmeHandler struct {
	programmes *programmeRepository
}

type Programme struct {
	ID            uuid.UUID `json:"id" format:"uuid"`
	Faculty       string    `json:"faculty" example:"Wydział Matematyki, Fizyki i Informatyki"`
	Programme     string    `json:"programme" example:"Informatyka"`
	Level         *string   `json:"level" nullable:"true" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school"`
	ModeOfStudies *string   `json:"modeOfStudies" nullable:"true" enum:"full_time,part_time"`
	YearOfStudy   *int      `json:"yearOfStudy" nullable:"true" minimum:"1" maximum:"6" example:"2"`
	Status        string    `json:"status" enum:"active,graduated,suspended,cancelled"`
	IsPrimary     bool      `json:"isPrimary"`
}

func toProgramme(row programmeRow) Programme {
	return Programme(row)
}

type programmeRequest struct {
	Faculty       string  `json:"faculty" minLength:"1" maxLength:"255"`
	Programme     string  `json:"programme" minLength:"1" maxLength:"255"`
	Level         *string `json:"level,omitempty" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school"`
	ModeOfStudies *string `json:"modeOfStudies,omitempty" enum:"full_time,part_time"`
	YearOfStudy   *int    `json:"yearOfStudy,omitempty" minimum:"1" maximum:"6"`
	Status        string  `json:"status,omitempty" enum:"active,graduated,suspended,cancelled" default:"active"`
	IsPrimary     bool    `json:"isPrimary,omitempty" doc:"Making a programme primary clears the flag on the others"`
}

func (r programmeRequest) fields() (programmeFields, error) {
	f := programmeFields{
		Faculty:       strings.TrimSpace(r.Faculty),
		Programme:     strings.TrimSpace(r.Programme),
		Level:         r.Level,
		ModeOfStudies: r.ModeOfStudies,
		YearOfStudy:   r.YearOfStudy,
		Status:        r.Status,
		IsPrimary:     r.IsPrimary,
	}
	if f.Faculty == "" || f.Programme == "" {
		return programmeFields{}, ErrInvalidInput
	}
	if f.Status == "" {
		f.Status = "active"
	}
	return f, nil
}

type listProgrammesOutput struct {
	Body struct {
		Programmes []Programme `json:"programmes"`
	}
}

var listProgrammesOperation = auth.Protected(huma.Operation{
	OperationID: "listMyProgrammes",
	Method:      http.MethodGet,
	Path:        "/profile/programmes",
	Summary:     "List the logged-in student's study programmes",
	Tags:        []string{"profile"},
})

func (h *programmeHandler) listProgrammes(ctx context.Context, _ *struct{}) (*listProgrammesOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	rows, err := h.programmes.list(ctx, userID)
	if err != nil {
		return nil, apiError(err)
	}

	out := &listProgrammesOutput{}
	out.Body.Programmes = make([]Programme, 0, len(rows))
	for _, row := range rows {
		out.Body.Programmes = append(out.Body.Programmes, toProgramme(row))
	}
	return out, nil
}

type createProgrammeInput struct {
	Body programmeRequest
}

type createProgrammeOutput struct {
	Body Programme
}

var createProgrammeOperation = auth.Protected(huma.Operation{
	OperationID:   "createMyProgramme",
	Method:        http.MethodPost,
	Path:          "/profile/programmes",
	Summary:       "Add a study programme",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest},
})

func (h *programmeHandler) createProgramme(ctx context.Context, in *createProgrammeInput) (*createProgrammeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	f, err := in.Body.fields()
	if err != nil {
		return nil, apiError(err)
	}
	row, err := h.programmes.create(ctx, userID, f)
	if err != nil {
		return nil, apiError(err)
	}
	return &createProgrammeOutput{Body: toProgramme(*row)}, nil
}

type replaceProgrammeInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body programmeRequest
}

type replaceProgrammeOutput struct {
	Body Programme
}

var replaceProgrammeOperation = auth.Protected(huma.Operation{
	OperationID: "replaceMyProgramme",
	Method:      http.MethodPut,
	Path:        "/profile/programmes/{id}",
	Summary:     "Replace a study programme",
	Description: "Overwrites every field; omitted optional fields become null.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
})

func (h *programmeHandler) replaceProgramme(ctx context.Context, in *replaceProgrammeInput) (*replaceProgrammeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	f, err := in.Body.fields()
	if err != nil {
		return nil, apiError(err)
	}
	row, err := h.programmes.replace(ctx, userID, in.ID, f)
	if err != nil {
		return nil, apiError(err)
	}
	return &replaceProgrammeOutput{Body: toProgramme(*row)}, nil
}

type deleteProgrammeInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

var deleteProgrammeOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteMyProgramme",
	Method:        http.MethodDelete,
	Path:          "/profile/programmes/{id}",
	Summary:       "Delete a study programme",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
})

func (h *programmeHandler) deleteProgramme(ctx context.Context, in *deleteProgrammeInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	if err := h.programmes.delete(ctx, userID, in.ID); err != nil {
		return nil, apiError(err)
	}
	return nil, nil
}
