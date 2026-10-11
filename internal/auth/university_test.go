package auth

import (
	"errors"
	"testing"
)

func TestGetUniversities(t *testing.T) {
	list := GetUniversities()

	expectedCount := 7
	if len(list) != expectedCount {
		t.Fatalf("expected %d universities, got %d", expectedCount, len(list))
	}

	if list[0].ID != UniversityUMCS || list[1].ID != UniversityUMLub {
		t.Errorf("unexpected ordering of universities: first is %s, second is %s", list[0].ID, list[1].ID)
	}
}

func TestUniversityAuthTypesMapping(t *testing.T) {
	tests := []struct {
		name         string
		id           UniversityID
		expectedAuth AuthType
	}{
		{name: "UMCS uses USOS", id: UniversityUMCS, expectedAuth: AuthTypeUSOS},
		{name: "UMLub uses USOS", id: UniversityUMLub, expectedAuth: AuthTypeUSOS},
		{name: "KUL uses OTP", id: UniversityKUL, expectedAuth: AuthTypeOTP},
		{name: "UP uses OTP", id: UniversityUP, expectedAuth: AuthTypeOTP},
		{name: "PolLub uses OTP", id: UniversityPolLub, expectedAuth: AuthTypeOTP},
		{name: "WSPA uses OTP", id: UniversityWSPA, expectedAuth: AuthTypeOTP},
		{name: "WSEI uses OTP", id: UniversityWSEI, expectedAuth: AuthTypeOTP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uni, err := GetUniversityByID(tt.id)
			if err != nil {
				t.Fatalf("unexpected error fetching %s: %v", tt.id, err)
			}

			if uni.AuthType != tt.expectedAuth {
				t.Errorf("university %s: expected AuthType %q, got %q", tt.id, tt.expectedAuth, uni.AuthType)
			}
		})
	}
}

func TestGetUniversityByID_NotFound(t *testing.T) {
	invalidID := UniversityID("unknown-uni")

	_, err := GetUniversityByID(invalidID)
	if err == nil {
		t.Fatal("expected error for non-existent university ID, got nil")
	}

	if !errors.Is(err, ErrUnknownUniversity) {
		t.Errorf("expected error to be ErrUnknownUniversity, got %v", err)
	}
}
