package push

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type tokenHandler struct {
	push Module
}

type registerMyPushTokenInput struct {
	Body struct {
		Token string `json:"token" maxLength:"200" doc:"The Expo push token of this device" example:"ExponentPushToken[xxxxxxxxxxxxxxxxxxxxxx]"`
	}
}

var registerMyPushTokenOperation = auth.Authenticated(huma.Operation{
	OperationID:   "registerMyPushToken",
	Method:        http.MethodPut,
	Path:          "/push/tokens/me",
	Summary:       "Set the push token of the logged-in student's current device",
	Description:   "One token per session: a new token replaces the old one, and a token that another session held moves to this one. Call it after every sign-in and whenever Expo hands out a new token.",
	Tags:          []string{"push"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest},
})

func (h *tokenHandler) registerToken(ctx context.Context, in *registerMyPushTokenInput) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}
	sessionID, ok := auth.SessionIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.push.registerToken(ctx, tokenChange{UserID: userID, SessionID: sessionID, Token: in.Body.Token}); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}

var deleteMyPushTokenOperation = auth.Authenticated(huma.Operation{
	OperationID:   "deleteMyPushToken",
	Method:        http.MethodDelete,
	Path:          "/push/tokens/me",
	Summary:       "Remove the push token of the logged-in student's current device",
	Description:   "Call it before signing out. Removing a token that is already gone succeeds.",
	Tags:          []string{"push"},
	DefaultStatus: http.StatusNoContent,
})

func (h *tokenHandler) deleteToken(ctx context.Context, _ *struct{}) (*struct{}, error) {
	sessionID, ok := auth.SessionIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	if err := h.push.removeToken(ctx, sessionID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
