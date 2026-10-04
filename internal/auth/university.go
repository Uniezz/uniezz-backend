package auth

import (
	"fmt"
)

type AuthType string

const (
	AuthTypeUSOS AuthType = "USOS"
	AuthTypeOTP  AuthType = "OTP"
)

type UniversityID string

const (
	UniversityUMCS   UniversityID = "umcs"
	UniversityUMLub  UniversityID = "umlub"
	UniversityKUL    UniversityID = "kul"
	UniversityUP     UniversityID = "up"
	UniversityPolLub UniversityID = "pollub"
	UniversityWSPA   UniversityID = "wspa"
	UniversityWSEI   UniversityID = "wsei"
)

type University struct {
	ID       UniversityID `json:"id"`
	Name     string       `json:"name"`
	AuthType AuthType     `json:"authType"`
	Domains  []string     `json:"domains,omitempty"`
}

var orderedUniversityIDs = []UniversityID{
	UniversityUMCS,
	UniversityUMLub,
	UniversityKUL,
	UniversityUP,
	UniversityPolLub,
	UniversityWSPA,
	UniversityWSEI,
}

var universities = map[UniversityID]University{
	UniversityUMCS: {
		ID:       UniversityUMCS,
		Name:     "Uniwersytet Marii Curie-Skłodowskiej",
		AuthType: AuthTypeUSOS,
	},
	UniversityUMLub: {
		ID:       UniversityUMLub,
		Name:     "Uniwersytet Medyczny w Lublinie",
		AuthType: AuthTypeUSOS,
	},
	UniversityKUL: {
		ID:       UniversityKUL,
		Name:     "Katolicki Uniwersytet Lubelski Jana Pawła II",
		AuthType: AuthTypeOTP,
		Domains:  []string{"student.kul.pl"},
	},
	UniversityUP: {
		ID:       UniversityUP,
		Name:     "Uniwersytet Przyrodniczy w Lublinie",
		AuthType: AuthTypeOTP,
		Domains:  []string{"up.lublin.pl"},
	},
	UniversityPolLub: {
		ID:       UniversityPolLub,
		Name:     "Politechnika Lubelska",
		AuthType: AuthTypeOTP,
		Domains:  []string{"pollub.pl"},
	},
	UniversityWSPA: {
		ID:       UniversityWSPA,
		Name:     "Wyższa Szkoła Przedsiębiorczości i Administracji w Lublinie",
		AuthType: AuthTypeOTP,
		Domains:  []string{"wspa.pl"},
	},
	UniversityWSEI: {
		ID:       UniversityWSEI,
		Name:     "Wyższa Szkoła Ekonomii i Innowacji w Lublinie",
		AuthType: AuthTypeOTP,
		Domains:  []string{"wsei.lublin.pl"},
	},
}

func GetUniversities() []University {
	list := make([]University, 0, len(orderedUniversityIDs))
	for _, id := range orderedUniversityIDs {
		if u, exists := universities[id]; exists {
			list = append(list, u)
		}
	}
	return list
}

func GetUniversityByID(id UniversityID) (University, error) {
	u, exists := universities[id]
	if !exists {
		return University{}, fmt.Errorf("%w: %q", ErrUnknownUniversity, id)
	}
	return u, nil
}
