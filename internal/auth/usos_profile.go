package auth

import (
	"strings"

	"github.com/Uniezz/uniezz-backend/internal/usos"
)

var usosLevels = map[string]bool{
	"first_cycle":  true,
	"second_cycle": true,
	"long_cycle":   true,
	"third_cycle":  true,
	"phd_school":   true,
}

func initialProfileFromUSOS(u usos.User) InitialProfile {
	initial := InitialProfile{}
	if u.HasPhoto {
		initial.PhotoURL = u.PhotoURLs["400x400"]
	}

	for _, sp := range u.Programmes {
		if sp.Status != "active" {
			continue
		}
		p := sp.Programme
		faculty := ""
		if p.Faculty != nil {
			faculty = strings.TrimSpace(p.Faculty.Name.String())
		}
		name := strings.TrimSpace(p.Name.String())
		if faculty == "" || name == "" {
			continue
		}

		level := ""
		if usosLevels[p.Level] {
			level = p.Level
		}

		initial.Programmes = append(initial.Programmes, InitialProgramme{
			Faculty:       faculty,
			Programme:     name,
			Level:         level,
			ModeOfStudies: usosModeOfStudies(p.ModeOfStudies),
			IsPrimary:     sp.IsPrimary,
		})
	}

	ensureOnePrimary(initial.Programmes)
	return initial
}

func ensureOnePrimary(programmes []InitialProgramme) {
	primary := -1
	for i := range programmes {
		if programmes[i].IsPrimary && primary == -1 {
			primary = i
		}
		programmes[i].IsPrimary = false
	}
	if primary == -1 && len(programmes) > 0 {
		primary = 0
	}
	if primary >= 0 {
		programmes[primary].IsPrimary = true
	}
}

func usosModeOfStudies(d usos.LangDict) string {
	en := strings.ToLower(d.EN)
	pl := strings.ToLower(d.PL)
	switch {
	case strings.Contains(en, "part-time") || strings.HasPrefix(pl, "niestacjonarne"):
		return "part_time"
	case strings.Contains(en, "full-time") || strings.HasPrefix(pl, "stacjonarne"):
		return "full_time"
	default:
		return ""
	}
}
