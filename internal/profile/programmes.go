package profile

import (
	"context"
	"net/http"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type ProgrammeDTO struct {
	ID            string  `json:"id" format:"uuid"`
	Faculty       string  `json:"faculty" example:"Wydział Matematyki, Fizyki i Informatyki"`
	Programme     string  `json:"programme" example:"Informatyka"`
	Level         *string `json:"level" nullable:"true" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school"`
	ModeOfStudies *string `json:"modeOfStudies" nullable:"true" enum:"full_time,part_time"`
	YearOfStudy   *int    `json:"yearOfStudy" nullable:"true" minimum:"1" maximum:"6" example:"2"`
	Status        string  `json:"status" enum:"active,graduated,suspended,cancelled"`
	IsPrimary     bool    `json:"isPrimary"`
}

func toProgrammeDTO(p programme) ProgrammeDTO {
	return ProgrammeDTO{
		ID:            p.ID.String(),
		Faculty:       p.Faculty,
		Programme:     p.Programme,
		Level:         p.Level,
		ModeOfStudies: p.ModeOfStudies,
		YearOfStudy:   p.YearOfStudy,
		Status:        p.Status,
		IsPrimary:     p.IsPrimary,
	}
}

type programmeBody struct {
	Faculty       string  `json:"faculty" minLength:"1" maxLength:"255"`
	Programme     string  `json:"programme" minLength:"1" maxLength:"255"`
	Level         *string `json:"level,omitempty" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school"`
	ModeOfStudies *string `json:"modeOfStudies,omitempty" enum:"full_time,part_time"`
	YearOfStudy   *int    `json:"yearOfStudy,omitempty" minimum:"1" maximum:"6"`
	Status        string  `json:"status,omitempty" enum:"active,graduated,suspended,cancelled" default:"active"`
	IsPrimary     bool    `json:"isPrimary,omitempty" doc:"Making a programme primary clears the flag on the others"`
}

func (b programmeBody) fields() (programmeFields, error) {
	f := programmeFields{
		Faculty:       strings.TrimSpace(b.Faculty),
		Programme:     strings.TrimSpace(b.Programme),
		Level:         b.Level,
		ModeOfStudies: b.ModeOfStudies,
		YearOfStudy:   b.YearOfStudy,
		Status:        b.Status,
		IsPrimary:     b.IsPrimary,
	}
	if f.Faculty == "" || f.Programme == "" {
		return programmeFields{}, ErrInvalidInput
	}
	if f.Status == "" {
		f.Status = "active"
	}
	return f, nil
}

type programmeOutput struct {
	Body ProgrammeDTO
}

type programmesOutput struct {
	Body struct {
		Programmes []ProgrammeDTO `json:"programmes"`
	}
}

var listProgrammesOperation = auth.Protected(huma.Operation{
	OperationID: "listMyProgrammes",
	Method:      http.MethodGet,
	Path:        "/profile/programmes",
	Summary:     "List the logged-in student's study programmes",
	Tags:        []string{"profile"},
})

func (m Module) listProgrammes(ctx context.Context, _ *struct{}) (*programmesOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	programmes, err := m.repo.listProgrammes(ctx, userID)
	if err != nil {
		return nil, apiError(err)
	}

	out := &programmesOutput{}
	out.Body.Programmes = make([]ProgrammeDTO, 0, len(programmes))
	for _, p := range programmes {
		out.Body.Programmes = append(out.Body.Programmes, toProgrammeDTO(p))
	}
	return out, nil
}

type createProgrammeInput struct {
	Body programmeBody
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

func (m Module) createProgramme(ctx context.Context, in *createProgrammeInput) (*programmeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	f, err := in.Body.fields()
	if err != nil {
		return nil, apiError(err)
	}
	p, err := m.repo.createProgramme(ctx, userID, f)
	if err != nil {
		return nil, apiError(err)
	}
	return &programmeOutput{Body: toProgrammeDTO(*p)}, nil
}

type replaceProgrammeInput struct {
	ID   string `path:"id" format:"uuid"`
	Body programmeBody
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

func (m Module) replaceProgramme(ctx context.Context, in *replaceProgrammeInput) (*programmeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, apiError(ErrNotFound)
	}
	f, err := in.Body.fields()
	if err != nil {
		return nil, apiError(err)
	}
	p, err := m.repo.replaceProgramme(ctx, userID, id, f)
	if err != nil {
		return nil, apiError(err)
	}
	return &programmeOutput{Body: toProgrammeDTO(*p)}, nil
}

type deleteProgrammeInput struct {
	ID string `path:"id" format:"uuid"`
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

func (m Module) deleteProgramme(ctx context.Context, in *deleteProgrammeInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	id, err := uuid.Parse(in.ID)
	if err != nil {
		return nil, apiError(ErrNotFound)
	}
	if err := m.repo.deleteProgramme(ctx, userID, id); err != nil {
		return nil, apiError(err)
	}
	return nil, nil
}
