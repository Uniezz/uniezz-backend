package auth

import (
	"context"
	"log"
	"net/http"
	"net/url"

	"github.com/danielgtaylor/huma/v2"
)

type usosHandler struct {
	usos   *usosLogin
	logins *logins

	webRedirectURL    string // e.g. https://uniezz.pl/auth/done
	mobileRedirectURL string // e.g. uniezz://auth/callback
}

type redirectOutput struct {
	Status   int
	Location string `header:"Location"`
}

func found(location string) *redirectOutput {
	return &redirectOutput{Status: http.StatusFound, Location: location}
}

type usosStartInput struct {
	University UniversityID `query:"university" required:"true" doc:"University to log in to" example:"umcs"`
	Platform   Platform     `query:"platform" required:"true" enum:"web,mobile" doc:"App to send the browser back to when the login is done"`
}

var usosStartOperation = huma.Operation{
	OperationID: "startUsosLogin",
	Method:      http.MethodGet,
	Path:        "/auth/usos/start",
	Summary:     "Start a USOS login",
	Description: "Redirects to the university's USOS authorization page. " +
		"Errors after the platform is known are sent to the app as a redirect with ?error=<code>.",
	Tags:          []string{"auth"},
	DefaultStatus: http.StatusFound,
	Errors:        []int{http.StatusBadRequest},
}

func (h *usosHandler) start(ctx context.Context, in *usosStartInput) (*redirectOutput, error) {
	authorizeURL, err := h.usos.start(ctx, in.University, in.Platform)
	if err != nil {
		return h.redirectWithError(in.Platform, err), nil
	}
	return found(authorizeURL), nil
}

type usosCallbackInput struct {
	OAuthToken    string `query:"oauth_token" required:"true" minLength:"1" doc:"Request token, set by USOS"`
	OAuthVerifier string `query:"oauth_verifier" required:"true" minLength:"1" doc:"Verifier, set by USOS"`
}

var usosCallbackOperation = huma.Operation{
	OperationID: "finishUsosLogin",
	Method:      http.MethodGet,
	Path:        "/auth/usos/callback",
	Summary:     "Finish a USOS login",
	Description: "Called by USOS, not by the apps. Redirects to the app with ?code=<login code> " +
		"to pass to exchangeLoginCode, or with ?error=<code>. " +
		"Answers with a JSON error only when the login state is unknown, as there is no app to return to.",
	Tags:          []string{"auth"},
	DefaultStatus: http.StatusFound,
	Errors:        []int{http.StatusBadRequest},
}

func (h *usosHandler) callback(ctx context.Context, in *usosCallbackInput) (*redirectOutput, error) {
	id, platform, err := h.usos.finish(ctx, in.OAuthToken, in.OAuthVerifier)
	if err != nil {
		if platform == "" {
			return nil, apiError(err)
		}
		return h.redirectWithError(platform, err), nil
	}

	user, err := h.logins.signIn(ctx, id)
	if err != nil {
		return h.redirectWithError(platform, err), nil
	}

	code, err := h.logins.issueLoginCode(ctx, user.ID)
	if err != nil {
		return h.redirectWithError(platform, err), nil
	}

	return h.redirectToApp(platform, url.Values{"code": {code}}), nil
}

func (h *usosHandler) redirectWithError(platform Platform, err error) *redirectOutput {
	status, code := errorResponse(err)
	if status >= http.StatusInternalServerError {
		log.Printf("auth: usos login: %v", err)
	}
	return h.redirectToApp(platform, url.Values{"error": {code}})
}

func (h *usosHandler) redirectToApp(platform Platform, params url.Values) *redirectOutput {
	target := h.webRedirectURL
	if platform == PlatformMobile {
		target = h.mobileRedirectURL
	}
	return found(target + "?" + params.Encode())
}
