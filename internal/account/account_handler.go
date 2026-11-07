package account

import (
	"context"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/danielgtaylor/huma/v2"
)

type accountHandler struct {
	account Module
}

var deleteAccountOperation = auth.Protected(huma.Operation{
	OperationID: "deleteMyAccount",
	Method:      http.MethodDelete,
	Path:        "/profile/me",
	Summary:     "Delete the logged-in student's account",
	Description: "Signs the student out everywhere and frees their university login, so signing in again creates a new, empty account. " +
		"Posts, comments, likes, files and the profile are removed in the background. " +
		"Requires a sign-in within the last 15 minutes: on 403 reauthentication_required, sign in again and retry.",
	Tags:          []string{"profile"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusForbidden},
})

func (h *accountHandler) deleteAccount(ctx context.Context, _ *struct{}) (*struct{}, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, unauthorized()
	}
	signedInAt, _ := auth.SignedInAt(ctx)

	if err := h.account.requestDeletion(ctx, userID, signedInAt); err != nil {
		return nil, apiError(err)
	}
	return nil, nil
}
