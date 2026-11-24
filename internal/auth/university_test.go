package auth

import (
	"errors"
	"strings"
	"testing"
)

func validUniversityChange() universityChange {
	return universityChange{
		ID:        "wsei",
		Name:      " Wyższa Szkoła Ekonomii i Innowacji ",
		AuthType:  AuthTypeOTP,
		Position:  7,
		Domains:   []string{" StudentWSEI.pl ", "wsei.lublin.pl", "studentwsei.pl"},
		Faculties: []facultyChange{{Name: "Wydział Nauk o Zdrowiu", Address: "ul. Projektowa 4", Latitude: 51.24552, Longitude: 22.61056}},
	}
}

func TestUniversityChange_Fields(t *testing.T) {
	t.Run("cleans the name and lowercases, sorts and dedupes the domains", func(t *testing.T) {
		f, err := validUniversityChange().fields()
		if err != nil {
			t.Fatalf("fields: %v", err)
		}
		if f.Name != "Wyższa Szkoła Ekonomii i Innowacji" {
			t.Errorf("name = %q", f.Name)
		}
		if strings.Join(f.Domains, ",") != "studentwsei.pl,wsei.lublin.pl" {
			t.Errorf("domains = %v", f.Domains)
		}
		if len(f.Faculties) != 1 || f.Faculties[0].Latitude != 51.24552 {
			t.Errorf("faculties = %+v", f.Faculties)
		}
	})

	t.Run("a USOS university needs no domains", func(t *testing.T) {
		c := validUniversityChange()
		c.AuthType, c.Domains = AuthTypeUSOS, nil
		if _, err := c.fields(); err != nil {
			t.Errorf("fields: %v", err)
		}
	})

	for _, tc := range []struct {
		name   string
		change func(*universityChange)
	}{
		{"an uppercase id", func(c *universityChange) { c.ID = "WSEI" }},
		{"an empty id", func(c *universityChange) { c.ID = "" }},
		{"an id with a space", func(c *universityChange) { c.ID = "ws ei" }},
		{"a short name", func(c *universityChange) { c.Name = "ab" }},
		{"an unknown login method", func(c *universityChange) { c.AuthType = "SAML" }},
		{"a negative position", func(c *universityChange) { c.Position = -1 }},
		{"an OTP university without domains", func(c *universityChange) { c.Domains = nil }},
		{"a domain without a dot", func(c *universityChange) { c.Domains = []string{"localhost"} }},
		{"a domain with an at sign", func(c *universityChange) { c.Domains = []string{"jan@wsei.pl"} }},
		{"a faculty without an address", func(c *universityChange) { c.Faculties[0].Address = " " }},
		{"a faculty outside the globe", func(c *universityChange) { c.Faculties[0].Latitude = 91 }},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			c := validUniversityChange()
			tc.change(&c)
			if _, err := c.fields(); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("fields = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestUniversityRow_AcceptsEmail(t *testing.T) {
	kul := testUniversities[uniKUL]
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

	if testUniversities[uniUMCS].acceptsEmail("jan@umcs.pl") {
		t.Error("expected a USOS university without domains to accept no email")
	}
}
