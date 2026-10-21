package auth

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type universitiesHandler struct{}

type UniversityDTO struct {
	ID         string   `json:"id" doc:"Unique identifier of the university" example:"umcs"`
	Name       string   `json:"name" doc:"Display name of the university" example:"Uniwersytet Marii Curie-Skłodowskiej"`
	AuthMethod string   `json:"authMethod" doc:"Authentication method used by the university" example:"USOS"`
	Domains    []string `json:"domains,omitempty" doc:"Allowed student email domains for OTP authentication" example:"[\"student.kul.pl\"]"`
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
		})

	}
	return &UniversitiesResponse{Body: response}, nil
}
