package auth

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
)

type AuthType string

const (
	AuthTypeUSOS AuthType = "USOS"
	AuthTypeOTP  AuthType = "OTP"
)

type UniversityID string

type universityChange struct {
	ID                 UniversityID
	Name               string
	AuthType           AuthType
	Position           int
	RadonInstitutionID *uuid.UUID
	Domains            []string
	Faculties          []facultyChange
}

type facultyChange struct {
	Name      string
	Address   string
	Latitude  float64
	Longitude float64
}

func (c universityChange) fields() (universityFields, error) {
	var f universityFields
	var err error
	if f.ID, err = cleanUniversityID(c.ID); err != nil {
		return universityFields{}, err
	}
	if f.Name, err = cleanUniversityName(c.Name); err != nil {
		return universityFields{}, err
	}
	if c.AuthType != AuthTypeUSOS && c.AuthType != AuthTypeOTP {
		return universityFields{}, ErrInvalidInput
	}
	if c.Position < 0 {
		return universityFields{}, ErrInvalidInput
	}
	if f.Domains, err = cleanDomains(c.Domains); err != nil {
		return universityFields{}, err
	}
	if c.AuthType == AuthTypeOTP && len(f.Domains) == 0 {
		return universityFields{}, ErrInvalidInput
	}
	if len(c.Faculties) > maxFaculties {
		return universityFields{}, ErrInvalidInput
	}
	f.Faculties = make([]facultyRow, 0, len(c.Faculties))
	for _, fc := range c.Faculties {
		var fr facultyRow
		if fr.Name, err = cleanFacultyName(fc.Name); err != nil {
			return universityFields{}, err
		}
		if fr.Address, err = cleanFacultyAddress(fc.Address); err != nil {
			return universityFields{}, err
		}
		if fc.Latitude < -90 || fc.Latitude > 90 || fc.Longitude < -180 || fc.Longitude > 180 {
			return universityFields{}, ErrInvalidInput
		}
		fr.Latitude, fr.Longitude = fc.Latitude, fc.Longitude
		f.Faculties = append(f.Faculties, fr)
	}
	f.AuthType = c.AuthType
	f.Position = c.Position
	f.RadonInstitutionID = c.RadonInstitutionID
	return f, nil
}

func (m Module) listUniversities(ctx context.Context) ([]University, error) {
	rows, err := m.universities.list(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]University, 0, len(rows))
	for _, r := range rows {
		items = append(items, r.university())
	}
	return items, nil
}

func (m Module) university(ctx context.Context, id UniversityID) (*University, error) {
	row, err := m.universities.get(ctx, id)
	if err != nil {
		return nil, err
	}
	u := row.university()
	return &u, nil
}

func (m Module) createUniversity(ctx context.Context, c universityChange) (*University, error) {
	f, err := c.fields()
	if err != nil {
		return nil, err
	}
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.universities.create(ctx, f); err != nil {
			return err
		}
		if err := m.universities.addDomains(ctx, f.ID, f.Domains); err != nil {
			return err
		}
		return m.universities.addFaculties(ctx, f.ID, f.Faculties)
	})
	if err != nil {
		return nil, err
	}
	return m.university(ctx, f.ID)
}

func (m Module) replaceUniversity(ctx context.Context, c universityChange) (*University, error) {
	f, err := c.fields()
	if err != nil {
		return nil, err
	}
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.universities.update(ctx, f); err != nil {
			return err
		}
		if err := m.universities.clearDomains(ctx, f.ID); err != nil {
			return err
		}
		if err := m.universities.addDomains(ctx, f.ID, f.Domains); err != nil {
			return err
		}
		if err := m.universities.clearFaculties(ctx, f.ID); err != nil {
			return err
		}
		return m.universities.addFaculties(ctx, f.ID, f.Faculties)
	})
	if err != nil {
		return nil, err
	}
	return m.university(ctx, f.ID)
}

func (r universityRow) university() University {
	faculties := make([]Faculty, 0, len(r.Faculties))
	for _, f := range r.Faculties {
		faculties = append(faculties, Faculty{
			Name:     f.Name,
			Address:  f.Address,
			Location: GeoPoint{Latitude: f.Latitude, Longitude: f.Longitude},
		})
	}
	domains := make([]string, 0, len(r.Domains))
	domains = append(domains, r.Domains...)
	return University{
		ID:                 string(r.ID),
		Name:               r.Name,
		AuthMethod:         string(r.AuthType),
		Position:           r.Position,
		RadonInstitutionID: r.RadonInstitutionID,
		Domains:            domains,
		Faculties:          faculties,
	}
}

func (r universityRow) acceptsEmail(email string) bool {
	local, domain, ok := strings.Cut(normalizeEmail(email), "@")
	if !ok || local == "" || strings.Contains(domain, "@") {
		return false
	}
	return slices.Contains(r.Domains, domain)
}
