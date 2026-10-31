package profile

import (
	"context"
	"net/http"
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type publicProfileHandler struct {
	profile Module
}

type PublicProfile struct {
	ID           uuid.UUID         `json:"id" format:"uuid"`
	Email        *string           `json:"email" nullable:"true" doc:"Null unless the student shows it"`
	FirstName    *string           `json:"firstName" nullable:"true" example:"Jan"`
	LastName     *string           `json:"lastName" nullable:"true" example:"Kowalski"`
	UniversityID string            `json:"universityId" example:"umcs"`
	AvatarURL    *string           `json:"avatarUrl" nullable:"true"`
	Bio          *string           `json:"bio" nullable:"true"`
	Interests    []string          `json:"interests"`
	Programmes   []PublicProgramme `json:"programmes" doc:"Primary first"`
}

type PublicProgramme struct {
	Faculty       *string `json:"faculty" nullable:"true" doc:"Null when the student hides it"`
	Programme     string  `json:"programme" example:"Informatyka"`
	Level         *string `json:"level" nullable:"true" enum:"first_cycle,second_cycle,long_cycle,third_cycle,phd_school"`
	ModeOfStudies *string `json:"modeOfStudies" nullable:"true" enum:"full_time,part_time"`
	YearOfStudy   *int    `json:"yearOfStudy" nullable:"true" doc:"Null when unknown or hidden by the student"`
	Status        string  `json:"status" enum:"active,graduated,suspended,cancelled"`
	IsPrimary     bool    `json:"isPrimary"`
}

type ProfileSummary struct {
	ID           uuid.UUID `json:"id" format:"uuid"`
	FirstName    *string   `json:"firstName" nullable:"true" example:"Jan"`
	LastName     *string   `json:"lastName" nullable:"true" example:"Kowalski"`
	UniversityID string    `json:"universityId" example:"umcs"`
	AvatarURL    *string   `json:"avatarUrl" nullable:"true"`
}

type listProfilesInput struct {
	httpapi.PageQuery
	University string `query:"university" maxLength:"32" doc:"Only students of this university" example:"umcs"`
	Name       string `query:"name" maxLength:"100" doc:"Case-insensitive match on first and last name" example:"kowal"`
}

type listProfilesOutput struct {
	Body struct {
		Items      []ProfileSummary `json:"items"`
		NextCursor *string          `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

var listProfilesOperation = auth.Protected(huma.Operation{
	OperationID: "listProfiles",
	Method:      http.MethodGet,
	Path:        "/profiles",
	Summary:     "List students",
	Description: "Newest accounts first, paginated by cursor.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *publicProfileHandler) listProfiles(ctx context.Context, in *listProfilesInput) (*listProfilesOutput, error) {
	if _, ok := auth.UserIDFrom(ctx); !ok {
		return nil, unauthorized()
	}

	items, next, err := h.profile.listProfiles(ctx, profileQuery{
		UniversityID: in.University,
		Name:         strings.TrimSpace(in.Name),
		Cursor:       in.Cursor,
		Limit:        in.Limit,
	})
	if err != nil {
		return nil, apiError(err)
	}

	out := &listProfilesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

type getProfileInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getProfileOutput struct {
	Body PublicProfile
}

var getProfileOperation = auth.Protected(huma.Operation{
	OperationID: "getProfile",
	Method:      http.MethodGet,
	Path:        "/profiles/{id}",
	Summary:     "Get a student's public profile",
	Description: "Fields the student hides through privacy settings are null.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusNotFound},
})

func (h *publicProfileHandler) getProfile(ctx context.Context, in *getProfileInput) (*getProfileOutput, error) {
	if _, ok := auth.UserIDFrom(ctx); !ok {
		return nil, unauthorized()
	}

	p, err := h.profile.publicProfile(ctx, in.ID)
	if err != nil {
		return nil, apiError(err)
	}
	return &getProfileOutput{Body: *p}, nil
}
