package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type profileHandler struct {
	profile Module
}

type Profile struct {
	ID           uuid.UUID   `json:"id" format:"uuid" doc:"User ID"`
	Email        *string     `json:"email" nullable:"true" doc:"University email, null when USOS did not share it" example:"jan.kowalski@umcs.pl"`
	FirstName    *string     `json:"firstName" nullable:"true" example:"Jan"`
	LastName     *string     `json:"lastName" nullable:"true" example:"Kowalski"`
	Gender       *string     `json:"gender" nullable:"true" enum:"male,female" doc:"Set once; null until set. Visible to other students, with no privacy toggle."`
	UniversityID string      `json:"universityId" example:"umcs"`
	Programmes   []Programme `json:"programmes" doc:"Study programmes, primary first; empty when the student has none"`
	AvatarURL    *string     `json:"avatarUrl" nullable:"true" doc:"URL of the avatar image, null when the student has none"`
	Bio          *string     `json:"bio" nullable:"true" doc:"Plain text, up to 500 characters" example:"Second-year CS student, into climbing."`
	Interests    []string    `json:"interests" doc:"Up to 10 tags" example:"[\"climbing\",\"go\"]"`
	Privacy      Privacy     `json:"privacy"`
	CreatedAt    time.Time   `json:"createdAt"`
}

func (Profile) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	// Nullable enums must explicitly include null in OpenAPI 3.1.
	s.Properties["gender"].Enum = append(s.Properties["gender"].Enum, nil)
	return s
}

type getMeOutput struct {
	Body Profile
}

var getMeOperation = auth.Protected(huma.Operation{
	OperationID: "getMyProfile",
	Method:      http.MethodGet,
	Path:        "/profile/me",
	Summary:     "Get the logged-in student's profile",
	Description: "Same shape for every university and login method. Read from our database only.",
	Tags:        []string{"profile"},
})

func (h *profileHandler) getMe(ctx context.Context, _ *struct{}) (*getMeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	p, err := h.profile.profile(ctx, userID)
	if err != nil {
		return nil, apiError(err)
	}
	return &getMeOutput{Body: *p}, nil
}

// genderInput distinguishes an omitted field from an explicit null.
type genderInput struct {
	Set   bool
	Value *string
}

func (g *genderInput) UnmarshalJSON(data []byte) error {
	g.Set = true
	return json.Unmarshal(data, &g.Value)
}

func (genderInput) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: huma.TypeString, Nullable: true, Enum: []any{"male", "female", nil}}
}

type updateMeInput struct {
	Body struct {
		Gender        genderInput `json:"gender,omitzero" doc:"Accepts male or female only while the saved gender is null. Once set, any supplied gender (including null or the same value) returns 400 gender_immutable. Omit to leave unchanged; null before setting is a no-op. Visible to other students; no privacy toggle."`
		FirstName     *string     `json:"firstName,omitempty" minLength:"1" maxLength:"100"`
		LastName      *string     `json:"lastName,omitempty" minLength:"1" maxLength:"100"`
		Bio           *string     `json:"bio,omitempty" maxLength:"2000" doc:"Plain text; HTML is stripped and the result may be up to 500 characters. An empty string clears it."`
		Interests     *[]string   `json:"interests,omitempty" maxItems:"10" doc:"Replaces all interests: up to 10 tags of up to 30 characters, duplicates ignored"`
		Privacy       Privacy     `json:"privacy,omitempty"`
		AvatarMediaID *uuid.UUID  `json:"avatarMediaId,omitempty" format:"uuid" doc:"mediaId of an avatar uploaded through createMediaUpload; the previous avatar is deleted"`
	}
}

type updateMeOutput struct {
	Body Profile
}

var updateMeOperation = auth.Protected(huma.Operation{
	OperationID: "updateMyProfile",
	Method:      http.MethodPatch,
	Path:        "/profile/me",
	Summary:     "Update the logged-in student's profile",
	Description: "Changes only the fields that are present. Returns the updated profile.",
	Tags:        []string{"profile"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *profileHandler) updateMe(ctx context.Context, in *updateMeInput) (*updateMeOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	err := h.profile.updateProfile(ctx, userID, profileChange{
		Gender:        in.Body.Gender.Value,
		SetGender:     in.Body.Gender.Set,
		FirstName:     in.Body.FirstName,
		LastName:      in.Body.LastName,
		Bio:           in.Body.Bio,
		Interests:     in.Body.Interests,
		Privacy:       in.Body.Privacy,
		AvatarMediaID: in.Body.AvatarMediaID,
	})
	if err != nil {
		return nil, apiError(err)
	}

	p, err := h.profile.profile(ctx, userID)
	if err != nil {
		return nil, apiError(err)
	}
	return &updateMeOutput{Body: *p}, nil
}

var deleteAvatarOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteMyAvatar",
	Method:        http.MethodDelete,
	Path:          "/profile/avatar",
	Summary:       "Remove the logged-in student's avatar",
	Description:   "Clears the avatar and deletes its file. Succeeds when there is no avatar.",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusNoContent,
})

func (h *profileHandler) deleteAvatar(ctx context.Context, _ *struct{}) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	if err := h.profile.removeAvatar(ctx, userID); err != nil {
		return nil, apiError(err)
	}
	return nil, nil
}
