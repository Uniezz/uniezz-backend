package httpjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureLog redirects the standard logger into a buffer for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prevOutput, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	})

	return &buf
}

func decodeErrorBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, rec.Body.String())
	}
	return body
}

func TestWrite(t *testing.T) {
	rec := httptest.NewRecorder()

	Write(rec, http.StatusCreated, map[string]string{"token": "abc"})

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if body["token"] != "abc" {
		t.Errorf("expected token abc, got %q", body["token"])
	}
}

func TestError(t *testing.T) {
	testCases := []struct {
		name    string
		status  int
		code    string
		wantLog bool
	}{
		{name: "client error is not logged", status: http.StatusBadRequest, code: "invalid_input", wantLog: false},
		{name: "server error is logged", status: http.StatusInternalServerError, code: "internal_error", wantLog: true},
		{name: "upstream error is logged", status: http.StatusBadGateway, code: "provider_unavailable", wantLog: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLog(t)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/auth/otp/verify", nil)
			secret := errors.New("pq: connection to 10.0.0.5 refused")

			Error(rec, req, tc.status, tc.code, secret)

			if rec.Code != tc.status {
				t.Errorf("expected status %d, got %d", tc.status, rec.Code)
			}

			body := decodeErrorBody(t, rec)
			if len(body) != 1 || body["error"] != tc.code {
				t.Errorf(`expected body {"error": %q}, got %s`, tc.code, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Errorf("error detail leaked to the client: %s", rec.Body.String())
			}

			logged := logs.String()
			if tc.wantLog {
				if !strings.Contains(logged, "10.0.0.5") || !strings.Contains(logged, "/auth/otp/verify") {
					t.Errorf("expected log with path and error detail, got %q", logged)
				}
			} else if logged != "" {
				t.Errorf("expected no log for a client error, got %q", logged)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	type request struct {
		Email string `json:"email"`
	}

	t.Run("valid body", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"email":"jan@student.kul.pl"}`))

		var got request
		if err := Decode(rec, req, &got); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
		if got.Email != "jan@student.kul.pl" {
			t.Errorf("expected email jan@student.kul.pl, got %q", got.Email)
		}
	})

	invalid := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "malformed JSON", body: `{"email":`},
		{name: "wrong type", body: `{"email":42}`},
		{name: "unknown field", body: `{"email":"jan@student.kul.pl","admin":true}`},
		{name: "body over 1 MiB", body: `{"email":"` + strings.Repeat("a", maxBodyBytes) + `"}`},
	}

	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))

			var got request
			err := Decode(rec, req, &got)
			if !errors.Is(err, ErrInvalidBody) {
				t.Fatalf("expected ErrInvalidBody, got: %v", err)
			}
		})
	}
}
