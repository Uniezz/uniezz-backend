package profile

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type ProfileDTO struct {
	ID           string         `json:"id" format:"uuid" doc:"User ID"`
	Email        *string        `json:"email" nullable:"true" doc:"University email, null when USOS did not share it" example:"jan.kowalski@umcs.pl"`
	FirstName    *string        `json:"firstName" nullable:"true" example:"Jan"`
	LastName     *string        `json:"lastName" nullable:"true" example:"Kowalski"`
	UniversityID string         `json:"universityId" example:"umcs"`
	Programmes   []ProgrammeDTO `json:"programmes" doc:"Study programmes, primary first; empty when the student has none"`
	AvatarURL    *string        `json:"avatarUrl" nullable:"true" doc:"URL of the avatar image, null when the student has none"`
	CreatedAt    time.Time      `json:"createdAt"`
}

type meOutput struct {
	Body ProfileDTO
}

var getMeOperation = auth.Protected(huma.Operation{
	OperationID: "getMyProfile",
	Method:      http.MethodGet,
	Path:        "/profile/me",
	Summary:     "Get the logged-in student's profile",
	Description: "Same shape for every university and login method. Read from our database only.",
	Tags:        []string{"profile"},
})

func (m Module) getMe(ctx context.Context, _ *struct{}) (*meOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	dto, err := m.profile(ctx, userID)
	if err != nil {
		return nil, apiError(err)
	}
	return &meOutput{Body: *dto}, nil
}

func (m Module) profile(ctx context.Context, userID uuid.UUID) (*ProfileDTO, error) {
	u, err := m.repo.getUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	programmes, err := m.repo.listProgrammes(ctx, userID)
	if err != nil {
		return nil, err
	}

	dto := &ProfileDTO{
		ID:           u.ID.String(),
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		UniversityID: u.UniversityID,
		Programmes:   make([]ProgrammeDTO, 0, len(programmes)),
		AvatarURL:    m.avatarURL(ctx, u.AvatarMediaID),
		CreatedAt:    u.CreatedAt,
	}
	for _, p := range programmes {
		dto.Programmes = append(dto.Programmes, toProgrammeDTO(p))
	}
	return dto, nil
}

func (m Module) avatarURL(ctx context.Context, mediaID *uuid.UUID) *string {
	if mediaID == nil {
		return nil
	}
	url, err := m.media.URL(ctx, *mediaID)
	if err != nil {
		telemetry.ReportError(ctx, "resolve avatar URL", err, slog.String("media_id", mediaID.String()))
		return nil
	}
	return &url
}

type updateMeInput struct {
	Body struct {
		FirstName     *string `json:"firstName,omitempty" minLength:"1" maxLength:"100"`
		LastName      *string `json:"lastName,omitempty" minLength:"1" maxLength:"100"`
		AvatarMediaID *string `json:"avatarMediaId,omitempty" format:"uuid" doc:"mediaId of an avatar uploaded through createMediaUpload; the previous avatar is deleted"`
	}
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

func (m Module) updateMe(ctx context.Context, in *updateMeInput) (*meOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	firstName, err := trimmedName(in.Body.FirstName)
	if err != nil {
		return nil, apiError(err)
	}
	lastName, err := trimmedName(in.Body.LastName)
	if err != nil {
		return nil, apiError(err)
	}

	var avatarID *uuid.UUID
	if in.Body.AvatarMediaID != nil {
		id, err := m.claimAvatar(ctx, userID, *in.Body.AvatarMediaID)
		if err != nil {
			return nil, apiError(err)
		}
		avatarID = id
	}

	var previous *uuid.UUID
	err = m.repo.inTx(ctx, func(tx pgx.Tx) error {
		if err := m.repo.updateNames(ctx, tx, userID, firstName, lastName); err != nil {
			return err
		}
		if avatarID == nil {
			return nil
		}
		var err error
		previous, err = m.swapAvatar(ctx, tx, userID, avatarID)
		return err
	})
	if err != nil {
		return nil, apiError(err)
	}
	m.deleteMedia(ctx, previous)

	dto, err := m.profile(ctx, userID)
	if err != nil {
		return nil, apiError(err)
	}
	return &meOutput{Body: *dto}, nil
}

func trimmedName(name *string) (*string, error) {
	if name == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*name)
	if trimmed == "" {
		return nil, ErrInvalidInput
	}
	return &trimmed, nil
}

func (m Module) claimAvatar(ctx context.Context, userID uuid.UUID, raw string) (*uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, ErrInvalidAvatar
	}

	current, err := m.repo.getUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if current.AvatarMediaID != nil && *current.AvatarMediaID == id {
		return nil, nil
	}

	err = m.media.Claim(ctx, id, userID, media.KindAvatar)
	if errors.Is(err, media.ErrNotFound) || errors.Is(err, media.ErrUploadMismatch) {
		return nil, errors.Join(ErrInvalidAvatar, err)
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (m Module) swapAvatar(ctx context.Context, tx pgx.Tx, userID uuid.UUID, avatarID *uuid.UUID) (*uuid.UUID, error) {
	previous, err := m.repo.replaceAvatar(ctx, tx, userID, avatarID)
	if err != nil || previous == nil {
		return nil, err
	}
	if avatarID != nil && *previous == *avatarID {
		return nil, nil
	}

	err = m.media.MarkDeleting(ctx, tx, *previous)
	if errors.Is(err, media.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return previous, nil
}

func (m Module) deleteMedia(ctx context.Context, mediaID *uuid.UUID) {
	if mediaID == nil {
		return
	}
	if err := m.media.Delete(ctx, *mediaID); err != nil {
		slog.WarnContext(ctx, "delete replaced avatar, left for cleanup", slog.String("media_id", mediaID.String()), slog.Any("error", err))
	}
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

func (m Module) deleteAvatar(ctx context.Context, _ *struct{}) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}

	var previous *uuid.UUID
	err := m.repo.inTx(ctx, func(tx pgx.Tx) error {
		var err error
		previous, err = m.swapAvatar(ctx, tx, userID, nil)
		return err
	})
	if err != nil {
		return nil, apiError(err)
	}
	m.deleteMedia(ctx, previous)
	return nil, nil
}
