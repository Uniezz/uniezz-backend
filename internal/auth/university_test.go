package auth

import (
	"errors"
	"strings"
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

func TestUniversity_AcceptsEmail(t *testing.T) {
	kul, err := GetUniversityByID(UniversityKUL)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		email string
		want  bool
	}{
		{"jan@student.kul.pl", true},
		{"  Jan.Kowalski@Student.KUL.pl ", true},
		{"jan@kul.pl", false},
		{"jan@x.student.kul.pl", false},
		{"jan@student.kul.pl.evil.com", false},
		{"jan@evilstudent.kul.pl", false},
		{"@student.kul.pl", false},
		{"student.kul.pl", false},
		{"a@b@student.kul.pl", false},
		{"", false},
	} {
		if got := kul.acceptsEmail(tc.email); got != tc.want {
			t.Errorf("acceptsEmail(%q) = %v, want %v", tc.email, got, tc.want)
		}
	}
}

func TestUniversity_USOSAcceptsNoEmail(t *testing.T) {
	umcs, err := GetUniversityByID(UniversityUMCS)
	if err != nil {
		t.Fatal(err)
	}
	if umcs.acceptsEmail("jan@umcs.pl") {
		t.Error("expected a USOS university without domains to accept no email")
	}
}

func TestUniversities_FacultiesAreComplete(t *testing.T) {
	for _, u := range GetUniversities() {
		if len(u.Faculties) == 0 {
			t.Errorf("%s has no faculties", u.ID)
		}
		for _, f := range u.Faculties {
			if strings.TrimSpace(f.Name) == "" || strings.TrimSpace(f.Address) == "" {
				t.Errorf("%s: faculty %+v must have a name and an address", u.ID, f)
			}
		}
	}
}

func TestUniversities_FacultiesLocatedInLublin(t *testing.T) {
	const (
		minLat, maxLat = 51.1, 51.35
		minLng, maxLng = 22.4, 22.7
	)
	for _, u := range GetUniversities() {
		for _, f := range u.Faculties {
			lat, lng := f.Location.Latitude, f.Location.Longitude
			if lat < minLat || lat > maxLat || lng < minLng || lng > maxLng {
				t.Errorf("%s: %q at (%v, %v) is outside Lublin; latitude and longitude may be swapped", u.ID, f.Name, lat, lng)
			}
		}
	}
}

func TestUniversities_OTPHaveLowercaseDomains(t *testing.T) {
	for _, u := range GetUniversities() {
		if u.AuthType != AuthTypeOTP {
			continue
		}
		if len(u.Domains) == 0 {
			t.Errorf("%s uses OTP but has no domains", u.ID)
		}
		for _, d := range u.Domains {
			if d != strings.ToLower(strings.TrimSpace(d)) {
				t.Errorf("%s: domain %q must be lowercase and trimmed to match normalized emails", u.ID, d)
			}
		}
	}
}
