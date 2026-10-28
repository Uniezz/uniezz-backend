package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

type meHandler struct {
	logins *logins
}

type meInput struct {
	Authorization string `header:"Authorization" doc:"Bearer <session token>"`
}

type UserProfileDTO struct {
	ID           string     `json:"id" doc:"Unique identifier of the user" example:"1431"`
	FullName     string     `json:"fullName,omitempty" doc:"Display first and last name of the user" example:"Janusz Kowalski"`
	Email        string     `json:"email" doc:"User primary email address" example:"jan.kowalski@student.kul.pl"`
	UniversityID string     `json:"universityId,omitempty" doc:"ID of the university user belongs to" example:"kul"`
	Major        string     `json:"major,omitempty" doc:"Field of study (kierunek studiów)" example:"Informatyka"`
	CurrentYear  int        `json:"currentYear,omitempty" doc:"Current student year of study" example:"3"`
	Avatar       string     `json:"avatar,omitempty" doc:"URL to user profile avatar image" example:"https://cdn.uniezz.pl/avatars/1431.jpg"`
	TrustBadge   TrustBadge `json:"trustBadge" doc:"Verification level: verified (USOS), directory (Entra), or domain (email)" example:"verified"`
}

type MeResponse struct {
	Body UserProfileDTO
}

var meOperation = huma.Operation{
	OperationID: "getMe",
	Method:      http.MethodGet,
	Path:        "/auth/me",
	Summary:     "Get the list of information about requester",
	Description: "?",
	Tags:        []string{"auth"},
}

func toProfileDTO(u *User) UserProfileDTO {
	return UserProfileDTO{
		ID:           u.ID.String(),
		FullName:     strings.TrimSpace(valueOr(u.FirstName) + " " + valueOr(u.LastName)),
		Email:        valueOr(u.Email),
		UniversityID: string(u.UniversityID),
		Major:        valueOr(u.Major),
		CurrentYear:  valueOr(u.CurrentYear),
		Avatar:       valueOr(u.Avatar),
		TrustBadge:   trustBadgeOf(u),
	}
}

func valueOr[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p

}

func (h *meHandler) getMe(ctx context.Context, input *meInput) (*MeResponse, error) {
	userID, err := h.logins.authenticate(ctx, input.Authorization)
	if err != nil {
		return nil, apiError(err)
	}
	user, err := h.logins.users.GetByID(ctx, userID)
	if err != nil {
		return nil, apiError(err)
	}
	return &MeResponse{Body: toProfileDTO(user)}, nil

}
