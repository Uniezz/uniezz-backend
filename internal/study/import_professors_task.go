package study

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
)

const (
	importAllSchedule = "study-import-professors"
	importInterval    = 30 * 24 * time.Hour
	importAllTimeout  = 30 * time.Second
	importTimeout     = 2 * time.Minute
	importAttempts    = 3
	maxRetiredShare   = 0.3
)

type ImportProfessorsArgs struct {
	University auth.UniversityID `json:"university"`
}

func (ImportProfessorsArgs) Kind() string { return "study.import-professors.v1" }

type ImportAllProfessorsArgs struct{}

func (ImportAllProfessorsArgs) Kind() string { return "study.import-all-professors.v1" }

type radonInstitution struct {
	university auth.UniversityID
	id         uuid.UUID
}

func (m Module) importAllProfessors(ctx context.Context, _ ImportAllProfessorsArgs) error {
	var institutions []radonInstitution
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		institutions, err = m.professors.radonInstitutions(ctx)
		if err != nil {
			return err
		}
		for _, institution := range institutions {
			if err := m.jobs.Enqueue(ctx, ImportProfessorsArgs{University: institution.university}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(institutions) > 0 {
		m.jobs.Wake()
	}
	return nil
}

func (m Module) importProfessors(ctx context.Context, args ImportProfessorsArgs) error {
	institution, err := m.professors.radonInstitution(ctx, args.University)
	if errors.Is(err, ErrNotFound) {
		return jobs.Permanent(fmt.Errorf("no POL-on institution for university %q", args.University))
	}
	if err != nil {
		return err
	}
	return m.syncUniversity(ctx, m.radon, institution)
}

func (m Module) syncUniversity(ctx context.Context, client *radonClient, institution radonInstitution) error {
	employees, err := client.employees(ctx, institution.id)
	if err != nil {
		return err
	}
	people := professorsAt(employees, institution)
	if len(people) == 0 {
		return fmt.Errorf("radon listed no employees for %s", institution.university)
	}
	keep := make([]uuid.UUID, 0, len(people))
	for _, person := range people {
		keep = append(keep, person.ExternalID)
	}

	university := string(institution.university)
	var retired int64
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		active, retiring, err := m.professors.countRetiring(ctx, university, keep)
		if err != nil {
			return err
		}
		if active > 0 && float64(retiring) > maxRetiredShare*float64(active) {
			return fmt.Errorf("radon would retire %d of %d professors of %s", retiring, active, university)
		}
		if err := m.professors.upsertMany(ctx, people); err != nil {
			return err
		}
		retired, err = m.professors.retire(ctx, university, keep)
		return err
	})
	if err != nil {
		return err
	}

	attr := slog.String("university", university)
	telemetry.RecordGauge(ctx, "study.professors.imported", float64(len(people)), attr)
	telemetry.RecordGauge(ctx, "study.professors.retired", float64(retired), attr)
	return nil
}

func professorsAt(employees []radonEmployee, institution radonInstitution) []professorFields {
	seen := make(map[uuid.UUID]bool, len(employees))
	people := make([]professorFields, 0, len(employees))
	for _, employee := range employees {
		var externalID uuid.UUID
		for _, post := range employee.Posts {
			if post.InstitutionID == institution.id {
				externalID = post.EmployeeID
				break
			}
		}
		if externalID == uuid.Nil || employee.ID == "" || seen[externalID] {
			continue
		}
		name, ok := cleanProfessorName(employee.PersonalData.FirstName, employee.PersonalData.MiddleName, employee.PersonalData.LastName)
		if !ok {
			continue
		}
		seen[externalID] = true
		people = append(people, professorFields{
			ExternalID:   externalID,
			RadonID:      employee.ID,
			Name:         name,
			Degree:       cleanDegree(employee.CalculatedEduLevel),
			UniversityID: string(institution.university),
		})
	}
	return people
}
