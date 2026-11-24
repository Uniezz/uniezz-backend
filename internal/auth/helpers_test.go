package auth

import "context"

const (
	uniUMCS   UniversityID = "umcs"
	uniUMLub  UniversityID = "umlub"
	uniKUL    UniversityID = "kul"
	uniPolLub UniversityID = "pollub"
)

type fakeUniversities map[UniversityID]universityRow

func (f fakeUniversities) get(_ context.Context, id UniversityID) (universityRow, error) {
	u, ok := f[id]
	if !ok {
		return universityRow{}, ErrUniversityNotFound
	}
	return u, nil
}

var testUniversities = fakeUniversities{
	uniUMCS:   {ID: uniUMCS, Name: "Uniwersytet Marii Curie-Skłodowskiej", AuthType: AuthTypeUSOS},
	uniUMLub:  {ID: uniUMLub, Name: "Uniwersytet Medyczny w Lublinie", AuthType: AuthTypeUSOS},
	uniKUL:    {ID: uniKUL, Name: "Katolicki Uniwersytet Lubelski", AuthType: AuthTypeOTP, Domains: []string{"student.kul.pl"}},
	uniPolLub: {ID: uniPolLub, Name: "Politechnika Lubelska", AuthType: AuthTypeOTP, Domains: []string{"pollub.pl"}},
}
