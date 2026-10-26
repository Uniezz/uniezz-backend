package auth

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

type otpHandler struct {
	otp    *otpLogin
	logins *logins
}

type otpRequestInput struct {
	Body struct {
		University UniversityID `json:"university" minLength:"1" doc:"University to log in to" example:"kul"`
		Email      string       `json:"email" minLength:"3" maxLength:"255" doc:"University email address" example:"jan.kowalski@student.kul.pl"`
	}
}

var otpRequestOperation = huma.Operation{
	OperationID:   "requestOtpCode",
	Method:        http.MethodPost,
	Path:          "/auth/otp/request",
	Summary:       "Email a one-time login code",
	Description:   "Sends a 6-digit code to the university email. The code is valid for 10 minutes and a new request replaces the previous code.",
	Tags:          []string{"auth"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusBadGateway},
}

func (h *otpHandler) request(ctx context.Context, in *otpRequestInput) (*struct{}, error) {
	if err := h.otp.request(ctx, in.Body.University, in.Body.Email); err != nil {
		return nil, apiError(err)
	}
	return nil, nil
}

type otpVerifyInput struct {
	Body struct {
		University UniversityID `json:"university" minLength:"1" doc:"University to log in to" example:"kul"`
		Email      string       `json:"email" minLength:"3" maxLength:"255" doc:"Email the code was sent to" example:"jan.kowalski@student.kul.pl"`
		Code       string       `json:"code" pattern:"^[0-9]{6}$" doc:"Code from the email" example:"123456"`
	}
}

type otpVerifyOutput struct {
	Body IssuedSession
}

var otpVerifyOperation = huma.Operation{
	OperationID: "verifyOtpCode",
	Method:      http.MethodPost,
	Path:        "/auth/otp/verify",
	Summary:     "Log in with an emailed code",
	Description: "Returns a session. A code allows 5 attempts; after that, or after it expires, request a new one.",
	Tags:        []string{"auth"},
	Errors:      []int{http.StatusBadRequest, http.StatusUnauthorized},
}

func (h *otpHandler) verify(ctx context.Context, in *otpVerifyInput) (*otpVerifyOutput, error) {
	id, err := h.otp.verify(ctx, in.Body.University, in.Body.Email, in.Body.Code)
	if err != nil {
		return nil, apiError(err)
	}

	user, err := h.logins.signIn(ctx, id)
	if err != nil {
		return nil, apiError(err)
	}

	session, err := h.logins.issueSession(ctx, user.ID)
	if err != nil {
		return nil, apiError(err)
	}
	return &otpVerifyOutput{Body: session}, nil
}
