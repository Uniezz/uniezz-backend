package auth

import (
	"errors"
	"reflect"
	"testing"
)

func TestIdentity_Validate(t *testing.T) {
	testCases := []struct {
		name     string
		identity Identity
		wantErr  bool
	}{
		{
			name:     "USOS university with usos user id",
			identity: Identity{UniversityID: uniUMCS, UsosUserID: "123456"},
		},
		{
			name:     "USOS university with usos user id and email",
			identity: Identity{UniversityID: uniUMLub, UsosUserID: "123456", Email: "jan@umlub.pl"},
		},
		{
			name:     "OTP university with email",
			identity: Identity{UniversityID: uniKUL, Email: "jan@student.kul.pl"},
		},
		{
			// An email alone must not log someone into a USOS university:
			// it would create a second account next to their USOS one.
			name:     "USOS university with only email",
			identity: Identity{UniversityID: uniUMCS, Email: "jan@umcs.pl"},
			wantErr:  true,
		},
		{
			name:     "OTP university with only usos user id",
			identity: Identity{UniversityID: uniKUL, UsosUserID: "123456"},
			wantErr:  true,
		},
		{
			name:     "USOS university with no keys",
			identity: Identity{UniversityID: uniUMCS, FirstName: "Jan", LastName: "Kowalski"},
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.identity.validate(testUniversities[tc.identity.UniversityID])

			if !tc.wantErr {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidIdentity) {
				t.Fatalf("expected ErrInvalidIdentity, got: %v", err)
			}
		})
	}
}

func TestIdentity_Normalized(t *testing.T) {
	original := Identity{
		UniversityID: uniKUL,
		UsosUserID:   "123456",
		Email:        "  Jan.Kowalski@Student.KUL.pl \t",
		FirstName:    "  Jan ",
		LastName:     "\tKowalski  ",
	}

	got := original.normalized()

	want := Identity{
		UniversityID: uniKUL,
		UsosUserID:   "123456",
		Email:        "jan.kowalski@student.kul.pl",
		FirstName:    "Jan",
		LastName:     "Kowalski",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalized() mismatch:\nexpected %+v\ngot      %+v", want, got)
	}

	if original.Email != "  Jan.Kowalski@Student.KUL.pl \t" || original.FirstName != "  Jan " {
		t.Errorf("normalized() must not modify the original identity, got %+v", original)
	}
}

func TestIdentity_Normalized_KeepsEmptyFieldsEmpty(t *testing.T) {
	got := Identity{UniversityID: uniUMCS, UsosUserID: "123456"}.normalized()

	if got.Email != "" || got.FirstName != "" || got.LastName != "" {
		t.Errorf("expected empty fields to stay empty, got %+v", got)
	}
}
