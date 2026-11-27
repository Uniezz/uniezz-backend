package study

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/telemetry"
	"github.com/Uniezz/uniezz-backend/internal/text"
)

const (
	importProgrammesSchedule = "study-import-programmes"
	importProgrammesInterval = 30 * 24 * time.Hour
	maxProgrammeNameLength   = 300
	radonCourseRunning       = "3"
	radonCourseClosing       = "1"
)

var radonLevels = map[string]ProgrammeLevel{
	"1": ProgrammeLongCycle,
	"2": ProgrammeFirstCycle,
	"3": ProgrammeSecondCycle,
}

type ImportProgrammesArgs struct {
	University auth.UniversityID `json:"university"`
}

func (ImportProgrammesArgs) Kind() string { return "study.import-programmes.v1" }

type ImportAllProgrammesArgs struct{}

func (ImportAllProgrammesArgs) Kind() string { return "study.import-all-programmes.v1" }

func (m Module) importAllProgrammes(ctx context.Context, _ ImportAllProgrammesArgs) error {
	var institutions []radonInstitution
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		institutions, err = m.professors.radonInstitutions(ctx)
		if err != nil {
			return err
		}
		for _, institution := range institutions {
			if err := m.jobs.Enqueue(ctx, ImportProgrammesArgs{University: institution.university}); err != nil {
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

func (m Module) importProgrammes(ctx context.Context, args ImportProgrammesArgs) error {
	institution, err := m.professors.radonInstitution(ctx, args.University)
	if errors.Is(err, ErrNotFound) {
		return jobs.Permanent(fmt.Errorf("no POL-on institution for university %q", args.University))
	}
	if err != nil {
		return err
	}
	return m.syncProgrammes(ctx, m.radon, institution)
}

func (m Module) syncProgrammes(ctx context.Context, client *radonClient, institution radonInstitution) error {
	courses, err := client.courses(ctx, institution.id)
	if err != nil {
		return err
	}
	programmes := programmesOf(courses)
	if len(programmes) == 0 {
		return fmt.Errorf("radon listed no programmes for %s", institution.university)
	}

	university := string(institution.university)
	var retired int64
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		active, retiring, err := m.programmes.countRetiring(ctx, university, programmes)
		if err != nil {
			return err
		}
		if active > 0 && float64(retiring) > maxRetiredShare*float64(active) {
			return fmt.Errorf("radon would retire %d of %d programmes of %s", retiring, active, university)
		}
		if err := m.programmes.upsertMany(ctx, university, programmes); err != nil {
			return err
		}
		retired, err = m.programmes.retire(ctx, university, programmes)
		return err
	})
	if err != nil {
		return err
	}

	attr := slog.String("university", university)
	telemetry.RecordGauge(ctx, "study.programmes.imported", float64(len(programmes)), attr)
	telemetry.RecordGauge(ctx, "study.programmes.retired", float64(retired), attr)
	return nil
}

func programmesOf(courses []radonCourse) []programmeFields {
	programmes := make([]programmeFields, 0, len(courses))
	for _, course := range courses {
		if course.StatusCode != radonCourseRunning && course.StatusCode != radonCourseClosing {
			continue
		}
		level, ok := radonLevels[course.LevelCode]
		if !ok {
			continue
		}
		name := strings.Join(strings.Fields(text.Plain(course.Name, false)), " ")
		if name == "" || utf8.RuneCountInString(name) > maxProgrammeNameLength {
			continue
		}
		faculty := ""
		if len(course.OrganizationUnits) > 0 {
			full := course.OrganizationUnits[0].FullName
			faculty = strings.TrimSpace(full[strings.LastIndex(full, ";")+1:])
		}
		programmes = append(programmes, programmeFields{Name: name, Level: level, Faculty: faculty})
	}
	return programmes
}
