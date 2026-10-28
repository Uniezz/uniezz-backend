package auth

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type universitiesHandler struct{}

type GeoPointDTO struct {
	Latitude  float64 `json:"latitude" doc:"Latitude in decimal degrees (WGS 84)" example:"51.24513"`
	Longitude float64 `json:"longitude" doc:"Longitude in decimal degrees (WGS 84)" example:"22.54247"`
}

type FacultyDTO struct {
	Name     string      `json:"name" doc:"Display name of the faculty" example:"Wydział Chemii"`
	Address  string      `json:"address" doc:"Street address of the faculty building" example:"pl. Marii Curie-Skłodowskiej 2"`
	Location GeoPointDTO `json:"location" doc:"Geographic location of the faculty building"`
}

type UniversityDTO struct {
	ID         string       `json:"id" doc:"Unique identifier of the university" example:"umcs"`
	Name       string       `json:"name" doc:"Display name of the university" example:"Uniwersytet Marii Curie-Skłodowskiej"`
	AuthMethod string       `json:"authMethod" doc:"Authentication method used by the university" example:"USOS"`
	Domains    []string     `json:"domains,omitempty" doc:"Allowed student email domains for OTP authentication" example:"[\"student.kul.pl\"]"`
	Faculties  []FacultyDTO `json:"faculties" doc:"Faculties of the university with their locations"`
}

type UniversitiesResponse struct {
	Body []UniversityDTO
}

var universitiesOperation = huma.Operation{
	OperationID: "getUniversities",
	Method:      http.MethodGet,
	Path:        "/auth/universities",
	Summary:     "Get the list of supported universities",
	Description: "Lists every university the app supports, in display order, with how its students log in.",
	Tags:        []string{"auth"},
}

func (h *universitiesHandler) list(ctx context.Context, input *struct{}) (*UniversitiesResponse, error) {
	domainUniversities := GetUniversities()

	var response []UniversityDTO
	for _, u := range domainUniversities {
		response = append(response, UniversityDTO{
			ID:         string(u.ID),
			Name:       u.Name,
			AuthMethod: string(u.AuthType),
			Domains:    u.Domains,
			Faculties:  toFacultyDTOs(u.Faculties),
		})

	}
	return &UniversitiesResponse{Body: response}, nil
}

func toFacultyDTOs(faculties []Faculty) []FacultyDTO {
	dtos := make([]FacultyDTO, 0, len(faculties))
	for _, f := range faculties {
		dtos = append(dtos, FacultyDTO{
			Name:    f.Name,
			Address: f.Address,
			Location: GeoPointDTO{
				Latitude:  f.Location.Latitude,
				Longitude: f.Location.Longitude,
			},
		})
	}
	return dtos
}
