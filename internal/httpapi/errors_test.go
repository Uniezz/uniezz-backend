package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestErrorResponses(t *testing.T) {
	errKnown := errors.New("known")
	responses := ErrorResponses{{Err: errKnown, Status: http.StatusConflict, Code: "known"}}

	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{errKnown, http.StatusConflict, "known"},
		{fmt.Errorf("wrapped: %w", errKnown), http.StatusConflict, "known"},
		{errors.New("other"), http.StatusInternalServerError, "internal_error"},
	} {
		status, code := responses.Lookup(tc.err)
		if status != tc.status || code != tc.code {
			t.Errorf("Lookup(%v) = %d %q, want %d %q", tc.err, status, code, tc.status, tc.code)
		}
		var body *ErrorBody
		if !errors.As(responses.Error(tc.err), &body) || body.GetStatus() != tc.status || body.Code != tc.code {
			t.Errorf("Error(%v) = %+v", tc.err, body)
		}
	}

	var body *ErrorBody
	if !errors.As(Unauthorized(), &body) || body.GetStatus() != http.StatusUnauthorized || body.Code != "unauthorized" {
		t.Errorf("Unauthorized() = %+v", body)
	}
}
