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

type redeemInviteInput struct {
	Body struct {
		Code string `json:"code" minLength:"1" maxLength:"32" doc:"Seven-character invitation code from an /invite/{code} link" example:"A1B2C3D"`
	}
}

type redeemInviteOutput struct {
	Body struct {
		CoinsEarned int `json:"coinsEarned" minimum:"0" doc:"Uniezz coins this request paid you; 0 when this code was already applied" example:"50"`
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

var redeemInviteOperation = Protected(huma.Operation{
	OperationID:   "redeemInvite",
	Method:        http.MethodPost,
	Path:          "/auth/invites/redeem",
	Summary:       "Apply an invitation code to the logged-in student's account",
	Description:   "Works for accounts created in the last 7 days that have no inviter, and awards 100 coins to the inviter and 50 to you. Sending the applied code again changes nothing; a different code returns 409 referral_already_set.",
	Tags:          []string{"auth"},
	DefaultStatus: http.StatusOK,
	Errors:        []int{http.StatusBadRequest, http.StatusForbidden, http.StatusConflict},
})

func (h *inviteCodeHandler) redeemInvite(ctx context.Context, input *redeemInviteInput) (*redeemInviteOutput, error) {
	userID, ok := UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	earned, err := h.auth.redeemInvite(ctx, userID, inviteChange{Code: input.Body.Code})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &redeemInviteOutput{}
	out.Body.CoinsEarned = earned
	return out, nil
}
