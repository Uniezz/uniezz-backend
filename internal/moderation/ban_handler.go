package moderation

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type banHandler struct {
	moderation Module
}

type Ban struct {
	UserID   uuid.UUID  `json:"userId" format:"uuid"`
	BannedAt *time.Time `json:"bannedAt" nullable:"true" doc:"Null when the student is not banned"`
}

type banInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type getBanOutput struct {
	Body Ban
}

var getBanOperation = auth.Authorized(huma.Operation{
	OperationID: "getBan",
	Method:      http.MethodGet,
	Path:        "/moderation/users/{id}/ban",
	Summary:     "Get whether a student is banned",
	Description: "Moderators and admins only.",
	Tags:        []string{"moderation"},
	Errors:      []int{http.StatusNotFound},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *banHandler) getBan(ctx context.Context, in *banInput) (*getBanOutput, error) {
	ban, err := h.moderation.ban(ctx, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getBanOutput{Body: *ban}, nil
}

var banUserOperation = auth.Authorized(huma.Operation{
	OperationID:   "banUser",
	Method:        http.MethodPut,
	Path:          "/moderation/users/{id}/ban",
	Summary:       "Ban a student",
	Description:   "Ends every session of the student at once, refuses new sign-ins with 403 banned, and marks pending reports about the student reviewed. Banning again changes nothing. Moderators and admins cannot be banned: an admin revokes the role first.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound, http.StatusConflict},
}, auth.RoleModerator, auth.RoleAdmin)

func (h *banHandler) banUser(ctx context.Context, in *banInput) (*struct{}, error) {
	if err := h.moderation.banUser(ctx, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var unbanUserOperation = auth.Authorized(huma.Operation{
	OperationID:   "unbanUser",
	Method:        http.MethodDelete,
	Path:          "/moderation/users/{id}/ban",
	Summary:       "Lift a student's ban",
	Description:   "The student can sign in again, and sessions from before the ban work again until they expire. Unbanning a student who is not banned succeeds.",
	Tags:          []string{"moderation"},
	DefaultStatus: http.StatusNoContent,
}, auth.RoleModerator, auth.RoleAdmin)

func (h *banHandler) unbanUser(ctx context.Context, in *banInput) (*struct{}, error) {
	if err := h.moderation.unbanUser(ctx, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
