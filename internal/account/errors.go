package account

import (
	"errors"
	"net/http"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
)

var ErrReauthenticationRequired = errors.New("sign in again to delete the account")

var errorResponses = []struct {
	err    error
	status int
	code   string
}{
	{ErrReauthenticationRequired, http.StatusForbidden, "reauthentication_required"},
}

func apiError(err error) error {
	for _, e := range errorResponses {
		if errors.Is(err, e.err) {
			return httpapi.Error(e.status, e.code, err)
		}
	}
	return httpapi.Error(http.StatusInternalServerError, "internal_error", err)
}

func unauthorized() error {
	return httpapi.Error(http.StatusUnauthorized, "unauthorized", nil)
}
