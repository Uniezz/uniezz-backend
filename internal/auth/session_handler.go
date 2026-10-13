package auth

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type sessionHandler struct {
	logins *logins
}

type exchangeInput struct {
	Body struct {
		Code string `json:"code" minLength:"1" maxLength:"64" doc:"One-time login code from the auth redirect"`
	}
}

type exchangeOutput struct {
	Body IssuedSession
}

var exchangeOperation = huma.Operation{
	OperationID: "exchangeLoginCode",
	Method:      http.MethodPost,
	Path:        "/auth/exchange",
	Summary:     "Exchange a one-time login code for a session",
	Tags:        []string{"auth"},
	Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized},
}

func (h *sessionHandler) exchange(ctx context.Context, in *exchangeInput) (*exchangeOutput, error) {
	session, err := h.logins.exchange(ctx, in.Body.Code)
	if err != nil {
		return nil, apiError(err)
	}
	return &exchangeOutput{Body: session}, nil
}
