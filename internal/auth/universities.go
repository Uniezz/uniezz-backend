package auth

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

type UniversityDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	AuthMethod string `json:"authMethod"`
}

type UniversitiesResponse struct {
	Body []UniversityDTO
}

var universitiesOperation = huma.Operation{
	OperationID: "get-universities",
	Method:      "GET",
	Path:        "/auth/universities",
	Summary:     "Get the list of supported universities",
}

func (m Module) getUniversities(ctx context.Context, input *struct{}) (*UniversitiesResponse, error) {
	domainUniversities := GetUniversities()

	var response []UniversityDTO
	for _, u := range domainUniversities {
		response = append(response, UniversityDTO{
			ID:         string(u.ID),
			Name:       u.Name,
			AuthMethod: string(u.AuthType),
		})

	}
	return &UniversitiesResponse{Body: response}, nil
}
