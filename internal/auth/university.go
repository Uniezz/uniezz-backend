package auth

import (
	"fmt"
	"slices"
	"strings"
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

type GeoPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Faculty struct {
	Name     string   `json:"name"`
	Address  string   `json:"address"`
	Location GeoPoint `json:"location"`
}

type University struct {
	ID        UniversityID `json:"id"`
	Name      string       `json:"name"`
	AuthType  AuthType     `json:"authType"`
	Domains   []string     `json:"domains,omitempty"`
	Faculties []Faculty    `json:"faculties"`
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
		Faculties: []Faculty{
			{Name: "Wydział Artystyczny", Address: "al. Kraśnicka 2B", Location: GeoPoint{Latitude: 51.248, Longitude: 22.5222}},
			{Name: "Wydział Biologii i Biotechnologii", Address: "ul. Akademicka 19", Location: GeoPoint{Latitude: 51.24308, Longitude: 22.54206}},
			{Name: "Wydział Chemii", Address: "pl. Marii Curie-Skłodowskiej 2", Location: GeoPoint{Latitude: 51.24513, Longitude: 22.54247}},
			{Name: "Wydział Ekonomiczny", Address: "pl. Marii Curie-Skłodowskiej 5", Location: GeoPoint{Latitude: 51.24564, Longitude: 22.54028}},
			{Name: "Wydział Filologiczny", Address: "pl. Marii Curie-Skłodowskiej 4A", Location: GeoPoint{Latitude: 51.24466, Longitude: 22.54097}},
			{Name: "Wydział Filozofii i Socjologii", Address: "pl. Marii Curie-Skłodowskiej 4", Location: GeoPoint{Latitude: 51.24525, Longitude: 22.54085}},
			{Name: "Wydział Historii i Archeologii", Address: "pl. Marii Curie-Skłodowskiej 4A", Location: GeoPoint{Latitude: 51.24466, Longitude: 22.54097}},
			{Name: "Wydział Matematyki, Fizyki i Informatyki", Address: "ul. Radziszewskiego 10", Location: GeoPoint{Latitude: 51.2456, Longitude: 22.5433}},
			{Name: "Wydział Nauk o Ziemi i Gospodarki Przestrzennej", Address: "al. Kraśnicka 2D", Location: GeoPoint{Latitude: 51.24794, Longitude: 22.52358}},
			{Name: "Wydział Pedagogiki i Psychologii", Address: "ul. Głęboka 43", Location: GeoPoint{Latitude: 51.2467, Longitude: 22.5251}},
			{Name: "Wydział Politologii i Dziennikarstwa", Address: "ul. Głęboka 45", Location: GeoPoint{Latitude: 51.247, Longitude: 22.5243}},
			{Name: "Wydział Prawa i Administracji", Address: "pl. Marii Curie-Skłodowskiej 5", Location: GeoPoint{Latitude: 51.24555, Longitude: 22.5399}},
		},
	},
	UniversityUMLub: {
		ID:       UniversityUMLub,
		Name:     "Uniwersytet Medyczny w Lublinie",
		AuthType: AuthTypeUSOS,
		Faculties: []Faculty{
			{Name: "Wydział Lekarski", Address: "ul. Chodźki 19", Location: GeoPoint{Latitude: 51.26318, Longitude: 22.56379}},
			{Name: "Wydział Lekarsko-Dentystyczny", Address: "ul. Chodźki 19", Location: GeoPoint{Latitude: 51.26318, Longitude: 22.56379}},
			{Name: "Wydział Farmaceutyczny", Address: "ul. Chodźki 1", Location: GeoPoint{Latitude: 51.25963, Longitude: 22.56781}},
			{Name: "Wydział Nauk Medycznych", Address: "ul. Chodźki 7", Location: GeoPoint{Latitude: 51.259, Longitude: 22.566}},
			{Name: "Wydział Nauk o Zdrowiu", Address: "ul. Staszica 4/6", Location: GeoPoint{Latitude: 51.24859, Longitude: 22.56194}},
		},
	},
	UniversityKUL: {
		ID:       UniversityKUL,
		Name:     "Katolicki Uniwersytet Lubelski Jana Pawła II",
		AuthType: AuthTypeOTP,
		Domains:  []string{"student.kul.pl"},
		Faculties: []Faculty{
			{Name: "Wydział Teologii", Address: "Al. Racławickie 14", Location: GeoPoint{Latitude: 51.24795, Longitude: 22.54485}},
			{Name: "Wydział Prawa, Prawa Kanonicznego i Administracji", Address: "ul. Spokojna 1", Location: GeoPoint{Latitude: 51.2494, Longitude: 22.5542}},
			{Name: "Wydział Filozofii", Address: "Al. Racławickie 14", Location: GeoPoint{Latitude: 51.24795, Longitude: 22.54485}},
			{Name: "Wydział Nauk Humanistycznych", Address: "Al. Racławickie 14", Location: GeoPoint{Latitude: 51.24795, Longitude: 22.54485}},
			{Name: "Wydział Nauk Społecznych i Technicznych", Address: "Al. Racławickie 14", Location: GeoPoint{Latitude: 51.24795, Longitude: 22.54485}},
			{Name: "Wydział Medyczny", Address: "ul. Konstantynów 1I", Location: GeoPoint{Latitude: 51.23635, Longitude: 22.50083}},
		},
	},
	UniversityUP: {
		ID:       UniversityUP,
		Name:     "Uniwersytet Przyrodniczy w Lublinie",
		AuthType: AuthTypeOTP,
		Domains:  []string{"up.lublin.pl"},
		Faculties: []Faculty{
			{Name: "Wydział Agrobioinżynierii", Address: "ul. Akademicka 13", Location: GeoPoint{Latitude: 51.2438, Longitude: 22.54049}},
			{Name: "Wydział Medycyny Weterynaryjnej", Address: "ul. Głęboka 30", Location: GeoPoint{Latitude: 51.2409, Longitude: 22.5438}},
			{Name: "Wydział Nauk o Zwierzętach i Biogospodarki", Address: "ul. Akademicka 13", Location: GeoPoint{Latitude: 51.2438, Longitude: 22.54049}},
			{Name: "Wydział Ogrodnictwa i Architektury Krajobrazu", Address: "ul. Akademicka 13", Location: GeoPoint{Latitude: 51.2438, Longitude: 22.54049}},
			{Name: "Wydział Inżynierii Produkcji", Address: "ul. Głęboka 28", Location: GeoPoint{Latitude: 51.2409, Longitude: 22.5451}},
			{Name: "Wydział Nauk o Żywności i Biotechnologii", Address: "ul. Skromna 8", Location: GeoPoint{Latitude: 51.24938, Longitude: 22.51063}},
			{Name: "Wydział Biologii Środowiskowej", Address: "ul. Akademicka 13", Location: GeoPoint{Latitude: 51.2438, Longitude: 22.54049}},
		},
	},
	UniversityPolLub: {
		ID:       UniversityPolLub,
		Name:     "Politechnika Lubelska",
		AuthType: AuthTypeOTP,
		Domains:  []string{"pollub.pl"},
		Faculties: []Faculty{
			{Name: "Wydział Mechaniczny", Address: "ul. Nadbystrzycka 36", Location: GeoPoint{Latitude: 51.2371, Longitude: 22.5496}},
			{Name: "Wydział Elektrotechniki i Informatyki", Address: "ul. Nadbystrzycka 38A", Location: GeoPoint{Latitude: 51.23677, Longitude: 22.54877}},
			{Name: "Wydział Budownictwa i Architektury", Address: "ul. Nadbystrzycka 40", Location: GeoPoint{Latitude: 51.2346, Longitude: 22.5505}},
			{Name: "Wydział Inżynierii Środowiska i Energetyki", Address: "ul. Nadbystrzycka 40B", Location: GeoPoint{Latitude: 51.2342, Longitude: 22.5504}},
			{Name: "Wydział Zarządzania", Address: "ul. Nadbystrzycka 38", Location: GeoPoint{Latitude: 51.23525, Longitude: 22.54873}},
			{Name: "Wydział Matematyki i Informatyki Technicznej", Address: "ul. Nadbystrzycka 38", Location: GeoPoint{Latitude: 51.23522, Longitude: 22.54925}},
		},
	},
	UniversityWSPA: {
		ID:       UniversityWSPA,
		Name:     "Wyższa Szkoła Przedsiębiorczości i Administracji w Lublinie",
		AuthType: AuthTypeOTP,
		Domains:  []string{"wspa.pl"},
		Faculties: []Faculty{
			{Name: "Wyższa Szkoła Przedsiębiorczości i Administracji", Address: "ul. Bursaki 12", Location: GeoPoint{Latitude: 51.2702302, Longitude: 22.5689341}},
		},
	},
	UniversityWSEI: {
		ID:       UniversityWSEI,
		Name:     "Wyższa Szkoła Ekonomii i Innowacji w Lublinie",
		AuthType: AuthTypeOTP,
		Domains:  []string{"wsei.lublin.pl", "studentwsei.pl"},
		Faculties: []Faculty{
			{Name: "Wydział Administracji i Nauk Społecznych", Address: "ul. Projektowa 4", Location: GeoPoint{Latitude: 51.24552, Longitude: 22.61056}},
			{Name: "Wydział Nauk o Zdrowiu", Address: "ul. Projektowa 4", Location: GeoPoint{Latitude: 51.24552, Longitude: 22.61056}},
			{Name: "Wydział Psychologii i Pedagogiki", Address: "ul. Projektowa 4", Location: GeoPoint{Latitude: 51.24552, Longitude: 22.61056}},
			{Name: "Wydział Transportu i Informatyki", Address: "ul. Projektowa 4", Location: GeoPoint{Latitude: 51.24552, Longitude: 22.61056}},
		},
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

func (u University) acceptsEmail(email string) bool {
	local, domain, ok := strings.Cut(normalizeEmail(email), "@")
	if !ok || local == "" || strings.Contains(domain, "@") {
		return false
	}
	return slices.Contains(u.Domains, domain)
}
