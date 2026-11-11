package study

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/google/uuid"
)

const (
	minDifficulty = 1
	maxDifficulty = 5
	minSemester   = 1
	maxSemester   = 10
)

type courseChange struct {
	Name         string
	Description  string
	Difficulty   int
	UniversityID string
	Faculty      string
	Subject      string
	Semester     int
}

func (c courseChange) fields() (courseFields, error) {
	var f courseFields
	var err error
	if f.Name, err = cleanName(c.Name); err != nil {
		return courseFields{}, err
	}
	if f.Description, err = cleanDescription(c.Description); err != nil {
		return courseFields{}, err
	}
	if f.Faculty, err = cleanName(c.Faculty); err != nil {
		return courseFields{}, err
	}
	if f.Subject, err = cleanName(c.Subject); err != nil {
		return courseFields{}, err
	}
	if !validDifficulty(c.Difficulty) || c.Semester < minSemester || c.Semester > maxSemester {
		return courseFields{}, ErrInvalidInput
	}
	if _, err := auth.GetUniversityByID(auth.UniversityID(c.UniversityID)); err != nil {
		return courseFields{}, ErrInvalidInput
	}
	f.Difficulty = c.Difficulty
	f.UniversityID = c.UniversityID
	f.Semester = c.Semester
	return f, nil
}

type courseEditChange struct {
	Name        *string
	Description *string
	Difficulty  *int
}

func (c courseEditChange) fields() (courseEditFields, error) {
	var f courseEditFields
	if c.Name != nil {
		name, err := cleanName(*c.Name)
		if err != nil {
			return courseEditFields{}, err
		}
		f.Name = &name
	}
	if c.Description != nil {
		description, err := cleanDescription(*c.Description)
		if err != nil {
			return courseEditFields{}, err
		}
		f.Description = &description
	}
	if c.Difficulty != nil {
		if !validDifficulty(*c.Difficulty) {
			return courseEditFields{}, ErrInvalidInput
		}
		f.Difficulty = c.Difficulty
	}
	return f, nil
}

type courseQuery struct {
	ViewerID     uuid.UUID
	UniversityID string
	Faculty      string
	Subject      string
	Semester     int
	Cursor       string
	Limit        int
}

func (q courseQuery) filter() (courseFilter, error) {
	if q.UniversityID != "" {
		if _, err := auth.GetUniversityByID(auth.UniversityID(q.UniversityID)); err != nil {
			return courseFilter{}, ErrInvalidInput
		}
	}
	if q.Semester != 0 && (q.Semester < minSemester || q.Semester > maxSemester) {
		return courseFilter{}, ErrInvalidInput
	}
	faculty, err := cleanFilterValue(q.Faculty)
	if err != nil {
		return courseFilter{}, err
	}
	subject, err := cleanFilterValue(q.Subject)
	if err != nil {
		return courseFilter{}, err
	}
	after, err := decodePageCursor(q.Cursor)
	if err != nil {
		return courseFilter{}, err
	}
	return courseFilter{
		UniversityID: q.UniversityID,
		Faculty:      faculty,
		Subject:      subject,
		Semester:     q.Semester,
		After:        after,
		Limit:        q.Limit + 1,
	}, nil
}

func (m Module) createCourse(ctx context.Context, authorID uuid.UUID, change courseChange) (*Course, error) {
	fields, err := change.fields()
	if err != nil {
		return nil, err
	}

	var row *courseRow
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.allowCreate(ctx, authorID); err != nil {
			return err
		}
		row, err = m.courses.create(ctx, authorID, fields)
		return err
	})
	if err != nil {
		return nil, err
	}
	course := toCourse(*row, authorID)
	return &course, nil
}

func (m Module) getCourse(ctx context.Context, viewerID, id uuid.UUID) (*Course, error) {
	row, err := m.courses.get(ctx, id)
	if err != nil {
		return nil, err
	}
	course := toCourse(*row, viewerID)
	return &course, nil
}

func (m Module) listCourses(ctx context.Context, q courseQuery) ([]Course, *string, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, nil, err
	}
	rows, err := m.courses.list(ctx, filter)
	if err != nil {
		return nil, nil, err
	}
	rows, next, err := httpapi.Page(rows, q.Limit, func(last courseRow) any {
		return pageCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	})
	if err != nil {
		return nil, nil, err
	}
	courses := make([]Course, 0, len(rows))
	for _, row := range rows {
		courses = append(courses, toCourse(row, q.ViewerID))
	}
	return courses, next, nil
}

func (m Module) updateCourse(ctx context.Context, userID, id uuid.UUID, change courseEditChange) (*Course, error) {
	fields, err := change.fields()
	if err != nil {
		return nil, err
	}
	row, err := m.courses.update(ctx, id, userID, fields)
	if err != nil {
		return nil, err
	}
	course := toCourse(*row, userID)
	return &course, nil
}

func (m Module) rateCourse(ctx context.Context, userID, id uuid.UUID, rating int) (*Course, error) {
	if !validRating(rating) {
		return nil, ErrInvalidInput
	}

	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.courses.lock(ctx, id); err != nil {
			return err
		}
		old, found, err := m.courses.getRating(ctx, id, userID)
		if err != nil {
			return err
		}
		if err := m.courses.saveRating(ctx, id, userID, rating); err != nil {
			return err
		}
		added := 0
		if !found {
			added = 1
		}
		return m.courses.addToRating(ctx, id, added, rating-old)
	})
	if err != nil {
		return nil, err
	}
	return m.getCourse(ctx, userID, id)
}

func (m Module) removeCourseRating(ctx context.Context, userID, id uuid.UUID) error {
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		if err := m.courses.lock(ctx, id); err != nil {
			return err
		}
		old, found, err := m.courses.deleteRating(ctx, id, userID)
		if err != nil || !found {
			return err
		}
		return m.courses.addToRating(ctx, id, -1, -old)
	})
}

func validDifficulty(difficulty int) bool {
	return difficulty >= minDifficulty && difficulty <= maxDifficulty
}

func toCourse(row courseRow, viewerID uuid.UUID) Course {
	return Course{
		ID:            row.ID,
		Name:          row.Name,
		Description:   row.Description,
		Difficulty:    row.Difficulty,
		UniversityID:  row.UniversityID,
		Faculty:       row.Faculty,
		Subject:       row.Subject,
		Semester:      row.Semester,
		RatingAverage: ratingAverage(row.RatingSum, row.RatingCount),
		RatingCount:   row.RatingCount,
		IsMine:        row.AuthorID != nil && *row.AuthorID == viewerID,
	}
}
