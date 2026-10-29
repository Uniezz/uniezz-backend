package profile

import (
	"context"
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
	UniversityID string      `json:"universityId" example:"umcs"`
	Programmes   []Programme `json:"programmes" doc:"Study programmes, primary first; empty when the student has none"`
	AvatarURL    *string     `json:"avatarUrl" nullable:"true" doc:"URL of the avatar image, null when the student has none"`
	CreatedAt    time.Time   `json:"createdAt"`
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

type updateMeInput struct {
	Body struct {
		FirstName     *string    `json:"firstName,omitempty" minLength:"1" maxLength:"100"`
		LastName      *string    `json:"lastName,omitempty" minLength:"1" maxLength:"100"`
		AvatarMediaID *uuid.UUID `json:"avatarMediaId,omitempty" format:"uuid" doc:"mediaId of an avatar uploaded through createMediaUpload; the previous avatar is deleted"`
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
		FirstName:     in.Body.FirstName,
		LastName:      in.Body.LastName,
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
