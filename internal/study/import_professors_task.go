package study

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/google/uuid"
)

const (
	importInterval  = 30 * 24 * time.Hour
	importTimeout   = 2 * time.Minute
	importAttempts  = 3
	maxRetiredShare = 0.3
)

type ImportProfessorsArgs struct {
	University auth.UniversityID `json:"university"`
}

func (ImportProfessorsArgs) Kind() string { return "study.import-professors.v1" }

func (a ImportProfessorsArgs) scheduleName() string {
	return "study-import-professors-" + string(a.University)
}

type radonInstitution struct {
	university auth.UniversityID
	id         uuid.UUID
}

var radonInstitutions = []radonInstitution{
	{auth.UniversityUMCS, uuid.MustParse("a013df22-d5c5-4871-849c-69e8c6e7e912")},
	{auth.UniversityUMLub, uuid.MustParse("28f45cd0-ed86-4162-baf8-d0add700e59f")},
	{auth.UniversityKUL, uuid.MustParse("9d3664e2-2c7a-448a-9fb2-278de61e6136")},
	{auth.UniversityUP, uuid.MustParse("bc05e3e9-35c4-47a4-be34-d13dd35fd0bf")},
	{auth.UniversityPolLub, uuid.MustParse("337ae4a1-4e95-456f-bc53-d8d54b952a53")},
	{auth.UniversityWSPA, uuid.MustParse("a029c48a-4b25-4ee7-bae6-8296a405206b")},
	{auth.UniversityWSEI, uuid.MustParse("f4dd5e8d-c071-4ec2-a048-faf9c2aae76d")},
}

func (m Module) importProfessors(ctx context.Context, args ImportProfessorsArgs) error {
	for _, institution := range radonInstitutions {
		if institution.university == args.University {
			return m.syncUniversity(ctx, m.radon, institution)
		}
	}
	return jobs.Permanent(fmt.Errorf("no POL-on institution for university %q", args.University))
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
