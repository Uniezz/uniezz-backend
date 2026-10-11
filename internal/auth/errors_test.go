package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/httpjson"
)

func TestErrorResponse(t *testing.T) {
	testCases := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{ErrUnknownUniversity, http.StatusBadRequest, "unknown_university"},
		{ErrWrongAuthMethod, http.StatusBadRequest, "wrong_auth_method"},
		{ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
		{httpjson.ErrInvalidBody, http.StatusBadRequest, "invalid_input"},
		{ErrStateNotFound, http.StatusBadRequest, "login_expired"},
		{ErrInvalidCode, http.StatusUnauthorized, "invalid_code"},
		{ErrInvalidTokenString, http.StatusUnauthorized, "unauthorized"},
		{ErrSessionNotFound, http.StatusUnauthorized, "unauthorized"},
		{ErrNotStudent, http.StatusForbidden, "not_student"},
		{ErrRateLimited, http.StatusTooManyRequests, "rate_limited"},
		{ErrUpstream, http.StatusBadGateway, "provider_unavailable"},
		{ErrInvalidIdentity, http.StatusInternalServerError, "internal_error"},
		{errors.New("unexpected"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tc := range testCases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			// Flows always wrap errors with detail, so test the wrapped form too.
			for _, err := range []error{tc.err, fmt.Errorf("usos callback: %w", tc.err)} {
				status, code := errorResponse(err)
				if status != tc.wantStatus || code != tc.wantCode {
					t.Errorf("errorResponse(%q) = %d %q, want %d %q", err, status, code, tc.wantStatus, tc.wantCode)
				}
			}
		})
	}
}

// An invalid Identity is our bug, even when its cause is an unknown university,
// so it must be a 500 and not the 400 that ErrUnknownUniversity alone gives.
func TestErrorResponse_InvalidIdentityWinsOverItsCause(t *testing.T) {
	err := Identity{UniversityID: "unknown-uni", Email: "jan@student.kul.pl"}.validate()
	if !errors.Is(err, ErrUnknownUniversity) {
		t.Fatalf("precondition: expected the error to wrap ErrUnknownUniversity, got: %v", err)
	}

	status, code := errorResponse(err)
	if status != http.StatusInternalServerError || code != "internal_error" {
		t.Errorf("expected 500 internal_error, got %d %q", status, code)
	}
}

func TestErrorResponses_EveryAuthErrorIsMapped(t *testing.T) {
	sentinels := []error{
		ErrUnknownUniversity, ErrWrongAuthMethod, ErrInvalidInput, ErrStateNotFound,
		ErrInvalidCode, ErrNotStudent, ErrRateLimited, ErrUpstream, ErrInvalidIdentity,
		ErrInvalidTokenString, ErrSessionNotFound,
	}

	for _, sentinel := range sentinels {
		found := false
		for _, e := range errorResponses {
			if e.err == sentinel {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%q has no entry in errorResponses", sentinel)
		}
	}
}

func TestWriteError(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/auth/usos/callback", nil)

	writeError(rec, req, fmt.Errorf("student_status=1 for usos user 123456: %w", ErrNotStudent))

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["error"] != "not_student" {
		t.Errorf(`expected {"error":"not_student"}, got %s`, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "123456") {
		t.Errorf("error detail leaked to the client: %s", rec.Body.String())
	}
}
