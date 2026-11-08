package httpapi

import (
	"errors"
	"net/http"
)

type ErrorResponse struct {
	Err    error
	Status int
	Code   string
}

type ErrorResponses []ErrorResponse

func (r ErrorResponses) Lookup(err error) (status int, code string) {
	for _, e := range r {
		if errors.Is(err, e.Err) {
			return e.Status, e.Code
		}
	}
	return http.StatusInternalServerError, "internal_error"
}

func (r ErrorResponses) Error(err error) error {
	status, code := r.Lookup(err)
	return Error(status, code, err)
}

func Unauthorized() error {
	return Error(http.StatusUnauthorized, "unauthorized", nil)
}
