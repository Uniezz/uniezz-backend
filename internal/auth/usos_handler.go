package auth

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
)

type usosHandler struct {
	usos   *usosLogin
	logins *logins

	webRedirectURL    string // e.g. https://uniezz.pl/auth/done
	mobileRedirectURL string // e.g. uniezz://auth/callback
}

func (h *usosHandler) start(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	platform, err := parsePlatform(q.Get("platform"))
	if err != nil {
		writeError(w, r, err)
		return
	}

	authorizeURL, err := h.usos.start(r.Context(), UniversityID(q.Get("university")), platform)
	if err != nil {
		h.redirectWithError(w, r, platform, err)
		return
	}

	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

func (h *usosHandler) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	requestToken, verifier := q.Get("oauth_token"), q.Get("oauth_verifier")
	if requestToken == "" || verifier == "" {
		writeError(w, r, fmt.Errorf("%w: missing oauth_token or oauth_verifier", ErrInvalidInput))
		return
	}

	id, platform, err := h.usos.finish(r.Context(), requestToken, verifier)
	if err != nil {
		if platform == "" {
			writeError(w, r, err)
			return
		}
		h.redirectWithError(w, r, platform, err)
		return
	}

	user, err := h.logins.signIn(r.Context(), id)
	if err != nil {
		h.redirectWithError(w, r, platform, err)
		return
	}

	code, err := h.logins.issueLoginCode(r.Context(), user.ID)
	if err != nil {
		h.redirectWithError(w, r, platform, err)
		return
	}

	h.redirectToApp(w, r, platform, url.Values{"code": {code}})
}

func (h *usosHandler) redirectWithError(w http.ResponseWriter, r *http.Request, platform Platform, err error) {
	status, code := errorResponse(err)
	if status >= http.StatusInternalServerError {
		log.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	}
	h.redirectToApp(w, r, platform, url.Values{"error": {code}})
}

func (h *usosHandler) redirectToApp(w http.ResponseWriter, r *http.Request, platform Platform, params url.Values) {
	target := h.webRedirectURL
	if platform == PlatformMobile {
		target = h.mobileRedirectURL
	}
	http.Redirect(w, r, target+"?"+params.Encode(), http.StatusFound)
}
