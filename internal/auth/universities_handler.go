package auth

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type universitiesHandler struct {
	auth Module
}

type GeoPoint struct {
	Latitude  float64 `json:"latitude" doc:"Latitude in decimal degrees (WGS 84)" example:"51.24513"`
	Longitude float64 `json:"longitude" doc:"Longitude in decimal degrees (WGS 84)" example:"22.54247"`
}

type Faculty struct {
	ID       uuid.UUID `json:"id" format:"uuid" doc:"Stable identifier of the faculty; courses refer to it"`
	Name     string    `json:"name" doc:"Display name of the faculty" example:"Wydział Chemii"`
	Address  string    `json:"address" doc:"Street address of the faculty building" example:"pl. Marii Curie-Skłodowskiej 2"`
	Location GeoPoint  `json:"location" doc:"Geographic location of the faculty building"`
}

type University struct {
	ID                 string     `json:"id" doc:"Unique identifier of the university" example:"umcs"`
	Name               string     `json:"name" doc:"Display name of the university" example:"Uniwersytet Marii Curie-Skłodowskiej"`
	AuthMethod         string     `json:"authMethod" doc:"Authentication method used by the university" example:"USOS"`
	Position           int        `json:"position" doc:"Display order, lowest first" example:"1"`
	RadonInstitutionID *uuid.UUID `json:"radonInstitutionId" format:"uuid" nullable:"true" doc:"POL-on institution whose professors are imported; null when none are imported"`
	Domains            []string   `json:"domains" doc:"Allowed student email domains for OTP authentication; empty for USOS universities" example:"[\"student.kul.pl\"]"`
	Faculties          []Faculty  `json:"faculties" doc:"Faculties of the university with their locations"`
}

type facultyRequest struct {
	ID        *uuid.UUID `json:"id,omitempty" format:"uuid" doc:"On replace, an existing faculty of this university to keep and update; omit to add a new faculty"`
	Name      string     `json:"name" minLength:"1" maxLength:"500" doc:"3 to 200 characters after cleaning" example:"Wydział Chemii"`
	Address   string     `json:"address" minLength:"1" maxLength:"500" doc:"Up to 200 characters after cleaning" example:"pl. Marii Curie-Skłodowskiej 2"`
	Latitude  float64    `json:"latitude" minimum:"-90" maximum:"90" doc:"Latitude in decimal degrees (WGS 84)" example:"51.24513"`
	Longitude float64    `json:"longitude" minimum:"-180" maximum:"180" doc:"Longitude in decimal degrees (WGS 84)" example:"22.54247"`
}

type universityRequest struct {
	Name               string           `json:"name" minLength:"1" maxLength:"500" doc:"3 to 200 characters after cleaning" example:"Uniwersytet Marii Curie-Skłodowskiej"`
	AuthMethod         AuthType         `json:"authMethod" enum:"USOS,OTP" doc:"How students log in" example:"OTP"`
	Position           int              `json:"position" minimum:"0" maximum:"10000" doc:"Display order, lowest first" example:"1"`
	RadonInstitutionID *uuid.UUID       `json:"radonInstitutionId,omitempty" format:"uuid" doc:"POL-on institution whose professors are imported; omit to import none"`
	Domains            []string         `json:"domains" maxItems:"20" doc:"Student email domains accepted for OTP login, at least one for OTP universities" example:"[\"student.kul.pl\"]"`
	Faculties          []facultyRequest `json:"faculties" maxItems:"50" doc:"Faculties in display order"`
}

func (r universityRequest) change(id string) universityChange {
	faculties := make([]facultyChange, 0, len(r.Faculties))
	for _, f := range r.Faculties {
		faculties = append(faculties, facultyChange(f))
	}
	return universityChange{
		ID:                 UniversityID(id),
		Name:               r.Name,
		AuthType:           r.AuthMethod,
		Position:           r.Position,
		RadonInstitutionID: r.RadonInstitutionID,
		Domains:            r.Domains,
		Faculties:          faculties,
	}
}

type getUniversitiesOutput struct {
	Body []University
}

var universitiesOperation = huma.Operation{
	OperationID: "getUniversities",
	Method:      http.MethodGet,
	Path:        "/auth/universities",
	Summary:     "Get the list of supported universities",
	Description: "Lists every university the app supports, in display order, with how its students log in. The list is short by design and not paginated.",
	Tags:        []string{"auth"},
}

func (h *universitiesHandler) getUniversities(ctx context.Context, _ *struct{}) (*getUniversitiesOutput, error) {
	items, err := h.auth.listUniversities(ctx)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getUniversitiesOutput{Body: items}, nil
}

type universityInput struct {
	ID string `path:"id" maxLength:"32" example:"umcs"`
}

type universityOutput struct {
	Body University
}

var getUniversityOperation = huma.Operation{
	OperationID: "getUniversity",
	Method:      http.MethodGet,
	Path:        "/auth/universities/{id}",
	Summary:     "Get a supported university",
	Tags:        []string{"auth"},
	Errors:      []int{http.StatusNotFound},
}

func (h *universitiesHandler) getUniversity(ctx context.Context, in *universityInput) (*universityOutput, error) {
	u, err := h.auth.university(ctx, UniversityID(in.ID))
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &universityOutput{Body: *u}, nil
}

type createUniversityInput struct {
	Body struct {
		ID                 string           `json:"id" minLength:"1" maxLength:"32" pattern:"^[a-z0-9][a-z0-9-]*$" doc:"Lowercase slug that identifies the university; it cannot change" example:"kul"`
		Name               string           `json:"name" minLength:"1" maxLength:"500" doc:"3 to 200 characters after cleaning" example:"Katolicki Uniwersytet Lubelski Jana Pawła II"`
		AuthMethod         AuthType         `json:"authMethod" enum:"USOS,OTP" doc:"How students log in" example:"OTP"`
		Position           int              `json:"position" minimum:"0" maximum:"10000" doc:"Display order, lowest first" example:"3"`
		RadonInstitutionID *uuid.UUID       `json:"radonInstitutionId,omitempty" format:"uuid" doc:"POL-on institution whose professors are imported; omit to import none"`
		Domains            []string         `json:"domains" maxItems:"20" doc:"Student email domains accepted for OTP login, at least one for OTP universities" example:"[\"student.kul.pl\"]"`
		Faculties          []facultyRequest `json:"faculties" maxItems:"50" doc:"Faculties in display order"`
	}
}

var createUniversityOperation = Authorized(huma.Operation{
	OperationID:   "createUniversity",
	Method:        http.MethodPost,
	Path:          "/auth/universities",
	Summary:       "Add a supported university",
	Description:   "Students can log in to it right away. A domain belongs to one university only. Admins only.",
	Tags:          []string{"auth"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest, http.StatusConflict},
}, RoleAdmin)

func (h *universitiesHandler) createUniversity(ctx context.Context, in *createUniversityInput) (*universityOutput, error) {
	body := universityRequest{
		Name:               in.Body.Name,
		AuthMethod:         in.Body.AuthMethod,
		Position:           in.Body.Position,
		RadonInstitutionID: in.Body.RadonInstitutionID,
		Domains:            in.Body.Domains,
		Faculties:          in.Body.Faculties,
	}
	u, err := h.auth.createUniversity(ctx, body.change(in.Body.ID))
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &universityOutput{Body: *u}, nil
}

type replaceUniversityInput struct {
	ID   string `path:"id" maxLength:"32" example:"kul"`
	Body universityRequest
}

var replaceUniversityOperation = Authorized(huma.Operation{
	OperationID: "replaceUniversity",
	Method:      http.MethodPut,
	Path:        "/auth/universities/{id}",
	Summary:     "Replace a supported university's name, login method, domains and faculties",
	Description: "Every field is replaced, so send the full lists of domains and faculties. A faculty sent with its id keeps the id; one left out is removed, which fails with 409 faculty_in_use while courses refer to it. Admins only.",
	Tags:        []string{"auth"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound, http.StatusConflict},
}, RoleAdmin)

func (h *universitiesHandler) replaceUniversity(ctx context.Context, in *replaceUniversityInput) (*universityOutput, error) {
	u, err := h.auth.replaceUniversity(ctx, in.Body.change(in.ID))
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &universityOutput{Body: *u}, nil
}
