package auth

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type inviteCodeHandler struct {
	auth Module
}

type inviteCodeOutput struct {
	Body struct {
		Code string `json:"code" minLength:"7" maxLength:"7" doc:"Your seven-character invitation code" example:"A1B2C3D"`
	}
}

var getMyInviteCodeOperation = Protected(huma.Operation{
	OperationID:   "getMyInviteCode",
	Method:        http.MethodGet,
	Path:          "/auth/invites/me",
	Summary:       "Get the logged-in student's invitation code",
	Description:   "Use the code in the /invite/{code} link. New accounts created with it award 100 coins to you and 50 to the new student.",
	Tags:          []string{"auth"},
	DefaultStatus: http.StatusOK,
	Errors:        []int{},
})

func (h *inviteCodeHandler) getMyInviteCode(ctx context.Context, _ *struct{}) (*inviteCodeOutput, error) {
	userID, ok := UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	code, err := h.auth.inviteCode(ctx, userID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &inviteCodeOutput{}
	out.Body.Code = code
	return out, nil
}
