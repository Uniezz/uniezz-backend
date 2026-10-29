package auth

import (
	"reflect"
	"testing"

	"github.com/Uniezz/uniezz-backend/internal/usos"
)

func usosProgramme(status, name, level string, primary bool) usos.StudentProgramme {
	return usos.StudentProgramme{
		Status:    status,
		IsPrimary: primary,
		Programme: usos.Programme{
			Name:          usos.LangDict{PL: name},
			Level:         level,
			ModeOfStudies: usos.LangDict{PL: "niestacjonarne", EN: "part-time"},
			Faculty:       &usos.Faculty{Name: usos.LangDict{EN: "Faculty of Law"}},
		},
	}
}

func TestUsosSeed(t *testing.T) {
	t.Run("keeps active programmes from several faculties and one primary", func(t *testing.T) {
		u := usos.User{Programmes: []usos.StudentProgramme{
			usosProgramme("active", "Prawo", "long_cycle", false),
			usosProgramme("graduated_end_of_study", "Historia", "first_cycle", true),
			usosProgramme("active", "Informatyka", "first_cycle", true),
			usosProgramme("active", "Fizyka", "first_cycle", true),
		}}

		got := usosSeed(u).Programmes

		want := []ProgrammeSeed{
			{Faculty: "Faculty of Law", Programme: "Prawo", Level: "long_cycle", ModeOfStudies: "part_time"},
			{Faculty: "Faculty of Law", Programme: "Informatyka", Level: "first_cycle", ModeOfStudies: "part_time", IsPrimary: true},
			{Faculty: "Faculty of Law", Programme: "Fizyka", Level: "first_cycle", ModeOfStudies: "part_time"},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("programmes mismatch:\nexpected %+v\ngot      %+v", want, got)
		}
	})

	t.Run("marks the first programme primary when USOS marks none", func(t *testing.T) {
		got := usosSeed(usos.User{Programmes: []usos.StudentProgramme{
			usosProgramme("active", "A", "", false),
			usosProgramme("active", "B", "", false),
		}}).Programmes

		if !got[0].IsPrimary || got[1].IsPrimary {
			t.Errorf("expected only the first programme primary, got %+v", got)
		}
	})

	t.Run("drops unknown levels and programmes without a faculty", func(t *testing.T) {
		noFaculty := usosProgramme("active", "Bez wydziału", "first_cycle", false)
		noFaculty.Programme.Faculty = nil

		got := usosSeed(usos.User{Programmes: []usos.StudentProgramme{
			usosProgramme("active", "Podyplomowe", "postgraduate", false),
			noFaculty,
		}}).Programmes

		if len(got) != 1 || got[0].Level != "" {
			t.Errorf("expected one programme with no level, got %+v", got)
		}
	})

	t.Run("takes the photo only when the student shares it", func(t *testing.T) {
		urls := map[string]string{"400x400": "https://usos.test/p.jpg"}

		if got := usosSeed(usos.User{HasPhoto: true, PhotoURLs: urls}).PhotoURL; got != "https://usos.test/p.jpg" {
			t.Errorf("expected the 400x400 photo, got %q", got)
		}
		if got := usosSeed(usos.User{HasPhoto: false, PhotoURLs: urls}).PhotoURL; got != "" {
			t.Errorf("expected no photo, got %q", got)
		}
	})
}

func TestUsosModeOfStudies(t *testing.T) {
	for _, tc := range []struct {
		dict usos.LangDict
		want string
	}{
		{usos.LangDict{PL: "stacjonarne", EN: "full-time"}, "full_time"},
		{usos.LangDict{PL: "niestacjonarne", EN: "part-time"}, "part_time"},
		{usos.LangDict{PL: "niestacjonarne"}, "part_time"},
		{usos.LangDict{PL: "stacjonarne"}, "full_time"},
		{usos.LangDict{PL: "e-learning"}, ""},
	} {
		if got := usosModeOfStudies(tc.dict); got != tc.want {
			t.Errorf("usosModeOfStudies(%+v) = %q, want %q", tc.dict, got, tc.want)
		}
	}
}
