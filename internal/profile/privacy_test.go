package profile

import (
	"maps"
	"testing"
)

func TestPrivacy(t *testing.T) {
	t.Run("missing settings fall back to their defaults, unknown stored keys are dropped", func(t *testing.T) {
		got := Privacy{"showEmail": true, "removedSetting": true}.withDefaults()
		want := Privacy{"showEmail": true, "showFaculty": true, "showYear": true}
		if !maps.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("hidden fields are cleared", func(t *testing.T) {
		email, faculty, year := "jan@umcs.pl", "WMFiI", 2
		p := &PublicProfile{Email: &email, Programmes: []PublicProgramme{{Faculty: &faculty, YearOfStudy: &year}}}

		Privacy{"showYear": false}.apply(p)

		if p.Email != nil {
			t.Error("email is hidden by default")
		}
		if p.Programmes[0].YearOfStudy != nil {
			t.Error("year of study must be hidden")
		}
		if p.Programmes[0].Faculty == nil {
			t.Error("faculty is shown by default")
		}
	})

	t.Run("every setting is in the schema", func(t *testing.T) {
		s := Privacy{}.Schema(nil)
		for name := range privacySettings {
			if s.Properties[name] == nil {
				t.Errorf("setting %q missing from the schema", name)
			}
		}
	})
}
