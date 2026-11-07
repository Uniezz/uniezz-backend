package profile

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

func TestGenderInput(t *testing.T) {
	var empty updateMeInput
	data, err := json.Marshal(empty.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("empty update must omit gender, got %s", data)
	}

	for _, tc := range []struct {
		body  string
		set   bool
		value string
	}{
		{`{}`, false, ""}, {`{"gender":null}`, true, ""}, {`{"gender":"male"}`, true, "male"}, {`{"gender":"female"}`, true, "female"},
	} {
		var in updateMeInput
		if err := json.Unmarshal([]byte(`{"Body":`+tc.body+`}`), &in); err != nil {
			t.Fatal(err)
		}
		if in.Body.Gender.Set != tc.set {
			t.Errorf("%s: set = %v", tc.body, in.Body.Gender.Set)
		}
		if tc.value == "" {
			if in.Body.Gender.Value != nil {
				t.Errorf("%s: expected nil", tc.body)
			}
		} else if in.Body.Gender.Value == nil || *in.Body.Gender.Value != tc.value {
			t.Errorf("%s: unexpected value", tc.body)
		}
	}
	for _, value := range []string{"", "other", "Male", " female "} {
		if _, err := (profileChange{Gender: &value, SetGender: true}).fields(); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%q: expected invalid input, got %v", value, err)
		}
	}
}

func TestGender(t *testing.T) {
	e := newTestEnv(t)
	viewer := e.otpUser(t)
	for _, gender := range []string{"male", "female"} {
		t.Run(gender, func(t *testing.T) {
			u := e.otpUser(t)
			public := decode[map[string]any](t, e.api.Get("/profiles/"+u.id.String(), viewer.bearer), http.StatusOK)
			if value, ok := public["gender"]; !ok || value != nil {
				t.Fatalf("unset public gender must be null, got %v", public)
			}
			body := decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"gender": nil}), http.StatusOK)
			if body.Gender != nil {
				t.Fatal("unset gender must remain null")
			}
			for _, invalid := range []any{"", "other", "Male", " female ", 1, true, []string{"male"}, map[string]string{"value": "male"}} {
				expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"gender": invalid}), http.StatusBadRequest, "invalid_input")
			}
			body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"gender": gender}), http.StatusOK)
			if body.Gender == nil || *body.Gender != gender {
				t.Fatalf("expected %s, got %v", gender, body.Gender)
			}
			for _, value := range []any{"male", "female", nil} {
				expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"gender": value, "firstName": "Changed"}), http.StatusBadRequest, "gender_immutable")
			}
			body = decode[Profile](t, e.api.Get("/profile/me", u.bearer), http.StatusOK)
			if body.Gender == nil || *body.Gender != gender || *body.FirstName != "Jan" {
				t.Fatalf("rejected requests changed profile: %+v", body)
			}
			body = decode[Profile](t, e.api.Patch("/profile/me", u.bearer, map[string]any{"bio": "Still editable"}), http.StatusOK)
			if body.Gender == nil || *body.Gender != gender {
				t.Fatal("omitting gender must preserve it")
			}
			public = decode[map[string]any](t, e.api.Get("/profiles/"+u.id.String(), viewer.bearer), http.StatusOK)
			if public["gender"] != gender {
				t.Fatalf("public gender = %v, want %s", public["gender"], gender)
			}
			list := decode[struct {
				Items []map[string]any `json:"items"`
			}](t, e.api.Get("/profiles", viewer.bearer), http.StatusOK)
			found := false
			for _, item := range list.Items {
				if _, ok := item["gender"]; !ok {
					t.Fatal("student search must include gender, even when null")
				}
				if item["id"] == u.id.String() {
					found = true
					if item["gender"] != gender {
						t.Fatalf("search gender = %v, want %s", item["gender"], gender)
					}
				}
			}
			if !found {
				t.Fatal("student missing from search")
			}
			expectError(t, e.api.Patch("/profile/me", u.bearer, map[string]any{"privacy": map[string]bool{"showGender": true}}), http.StatusBadRequest, "invalid_input")
		})
	}
}

func TestGenderConcurrentSet(t *testing.T) {
	e := newTestEnv(t)
	u := e.otpUser(t)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, gender := range []string{"male", "female"} {
		go func() {
			<-start
			results <- e.module.updateProfile(context.Background(), u.id, profileChange{Gender: &gender, SetGender: true})
		}()
	}
	close(start)
	var saved, rejected int
	for range 2 {
		switch err := <-results; {
		case err == nil:
			saved++
		case errors.Is(err, ErrGenderImmutable):
			rejected++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if saved != 1 || rejected != 1 {
		t.Fatalf("saved=%d rejected=%d, want one each", saved, rejected)
	}
}
