package profile

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
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
	Gender       *Gender     `json:"gender" nullable:"true" enum:"male,female" doc:"Set once; null until set. Visible to other students, with no privacy toggle."`
	Language     Language    `json:"language" enum:"en,pl,uk,ru,be,de,es" doc:"Interface language; defaults to pl"`
	UniversityID string      `json:"universityId" example:"umcs"`
	Programmes   []Programme `json:"programmes" doc:"Study programmes, primary first; empty when the student has none"`
	AvatarURL    *string     `json:"avatarUrl" nullable:"true" doc:"URL of the avatar image, null when the student has none"`
	BannerURL    *string     `json:"bannerUrl" nullable:"true" doc:"URL of the banner image, null when the student has none"`
	Bio          *string     `json:"bio" nullable:"true" doc:"Plain text, up to 500 characters" example:"Second-year CS student, into climbing."`
	SongURL      *string     `json:"songUrl" nullable:"true" doc:"Link to the song the student associates with themselves. Null when unset." example:"https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT"`
	Interests    []string    `json:"interests" doc:"Up to 10 tags" example:"[\"climbing\",\"go\"]"`
	Privacy      Privacy     `json:"privacy"`
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
		return nil, httpapi.Unauthorized()
	}

	p, err := h.profile.profile(ctx, userID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getMeOutput{Body: *p}, nil
}

type updateMeInput struct {
	Body struct {
		FirstName     *string    `json:"firstName,omitempty" minLength:"1" maxLength:"100"`
		LastName      *string    `json:"lastName,omitempty" minLength:"1" maxLength:"100"`
		Gender        *Gender    `json:"gender,omitempty" enum:"male,female" doc:"Can be set only once: when a gender is already saved, returns 400 gender_immutable"`
		Language      *Language  `json:"language,omitempty" enum:"en,pl,uk,ru,be,de,es"`
		Bio           *string    `json:"bio,omitempty" maxLength:"2000" doc:"Plain text; HTML is stripped and the result may be up to 500 characters. An empty string clears it."`
		SongURL       *string    `json:"songUrl,omitempty" maxLength:"2000" doc:"http or https URL, up to 500 characters. An empty string clears it. Anything else returns 400 invalid_input and leaves the saved link unchanged."`
		Interests     *[]string  `json:"interests,omitempty" maxItems:"10" doc:"Replaces all interests: up to 10 tags of up to 30 characters, duplicates ignored"`
		Privacy       Privacy    `json:"privacy,omitempty"`
		AvatarMediaID *uuid.UUID `json:"avatarMediaId,omitempty" format:"uuid" doc:"mediaId of an avatar uploaded through createMediaUpload; the previous avatar is deleted"`
		BannerMediaID *uuid.UUID `json:"bannerMediaId,omitempty" format:"uuid" doc:"mediaId of a banner uploaded through createMediaUpload; the previous banner is deleted"`
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
		return nil, httpapi.Unauthorized()
	}

	err := h.profile.updateProfile(ctx, userID, profileChange{
		FirstName:     in.Body.FirstName,
		LastName:      in.Body.LastName,
		Gender:        in.Body.Gender,
		Language:      in.Body.Language,
		Bio:           in.Body.Bio,
		SongURL:       in.Body.SongURL,
		Interests:     in.Body.Interests,
		Privacy:       in.Body.Privacy,
		AvatarMediaID: in.Body.AvatarMediaID,
		BannerMediaID: in.Body.BannerMediaID,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}

	p, err := h.profile.profile(ctx, userID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &updateMeOutput{Body: *p}, nil
}

var deleteMeOperation = auth.Protected(huma.Operation{
	OperationID: "deleteMyProfile",
	Method:      http.MethodDelete,
	Path:        "/profile/me",
	Summary:     "Delete the logged-in student's profile and account",
	Description: "Signs the student out everywhere and frees their university login, so signing in again creates a new, empty account. " +
		"Posts, comments, likes, study files, uploads and the profile are removed in the background. " +
		"Requires a sign-in within the last 15 minutes: on 403 reauthentication_required, sign in again and retry.",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusForbidden},
})

func (h *profileHandler) deleteMe(ctx context.Context, _ *struct{}) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	signedInAt, _ := auth.SignedInAt(ctx)

	if err := h.profile.deleteAccount(ctx, userID, signedInAt); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
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
		return nil, httpapi.Unauthorized()
	}

	if err := h.profile.removeMedia(ctx, userID, avatarSlot); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var deleteBannerOperation = auth.Protected(huma.Operation{
	OperationID:   "deleteMyBanner",
	Method:        http.MethodDelete,
	Path:          "/profile/banner",
	Summary:       "Remove the logged-in student's banner",
	Description:   "Clears the banner and deletes its file. Succeeds when there is no banner.",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusNoContent,
})

func (h *profileHandler) deleteBanner(ctx context.Context, _ *struct{}) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.profile.removeMedia(ctx, userID, bannerSlot); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
