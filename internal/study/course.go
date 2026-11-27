package study

import (
	"context"

	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	minDifficulty     = 1
	maxDifficulty     = 5
	similarNameScore  = 0.45
	maxSimilarCourses = 5
)

type courseChange struct {
	Name        string
	Description string
	Difficulty  int
	FacultyID   uuid.UUID
	ConfirmNew  bool
}

func (c courseChange) fields() (courseFields, error) {
	var f courseFields
	var err error
	if f.Name, err = cleanName("name", c.Name); err != nil {
		return courseFields{}, err
	}
	if f.Description, err = cleanDescription(c.Description); err != nil {
		return courseFields{}, err
	}
	if !validDifficulty(c.Difficulty) || c.FacultyID == uuid.Nil {
		return courseFields{}, ErrInvalidInput
	}
	f.Difficulty = c.Difficulty
	f.FacultyID = c.FacultyID
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
		name, err := cleanName("name", *c.Name)
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
	FacultyID    uuid.UUID
	Search       string
	Cursor       string
	Limit        int
}

type similarCoursesQuery struct {
	ViewerID  uuid.UUID
	FacultyID uuid.UUID
	Name      string
}

func (q similarCoursesQuery) filter() (similarCoursesFilter, error) {
	name, err := cleanName("name", q.Name)
	if err != nil {
		return similarCoursesFilter{}, err
	}
	return similarCoursesFilter{
		ViewerID:  q.ViewerID,
		FacultyID: q.FacultyID,
		Name:      name,
		MinScore:  similarNameScore,
		Limit:     maxSimilarCourses,
	}, nil
}

func (q courseQuery) filter() (courseFilter, error) {
	words, err := text.SearchWords(q.Search)
	if err != nil {
		return courseFilter{}, ErrInvalidInput
	}
	after, err := decodePageCursor(q.Cursor)
	if err != nil {
		return courseFilter{}, err
	}
	return courseFilter{
		ViewerID:     q.ViewerID,
		UniversityID: q.UniversityID,
		FacultyID:    q.FacultyID,
		Words:        words,
		After:        after,
		Limit:        q.Limit + 1,
	}, nil
}

func (m Module) createCourse(ctx context.Context, authorID uuid.UUID, change courseChange) (*Course, error) {
	fields, err := change.fields()
	if err != nil {
		return nil, err
	}
	if !change.ConfirmNew {
		similar, err := m.courses.hasSimilar(ctx, fields.FacultyID, fields.Name, similarNameScore)
		if err != nil {
			return nil, err
		}
		if similar {
			return nil, ErrSimilarCourses
		}
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
	row, err := m.courses.get(ctx, viewerID, id)
	if err != nil {
		return nil, err
	}
	course := toCourse(*row, viewerID)
	return &course, nil
}

func (m Module) similarCourses(ctx context.Context, q similarCoursesQuery) ([]Course, error) {
	filter, err := q.filter()
	if err != nil {
		return nil, err
	}
	rows, err := m.courses.similar(ctx, filter)
	if err != nil {
		return nil, err
	}
	courses := make([]Course, 0, len(rows))
	for _, row := range rows {
		courses = append(courses, toCourse(row, q.ViewerID))
	}
	return courses, nil
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

func (m Module) deleteCourse(ctx context.Context, userID, id uuid.UUID, anyAuthor bool) error {
	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		found, err := m.courses.lockForDelete(ctx, id, userID, anyAuthor)
		if err != nil || !found {
			return err
		}
		mediaIDs, err := m.files.deleteByCourse(ctx, id)
		if err != nil {
			return err
		}
		if err := m.media.MarkDeletingMany(ctx, mediaIDs); err != nil {
			return err
		}
		if err := m.comments.deleteByCourse(ctx, id); err != nil {
			return err
		}
		return m.courses.delete(ctx, id)
	})
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
		if found {
			return m.courses.addToRating(ctx, id, 0, rating-old)
		}
		if err := m.courses.addToRating(ctx, id, 1, rating); err != nil {
			return err
		}
		_, err = m.rewards.Track(ctx, userID, rewards.ActionCourseRated)
		return err
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
		FacultyID:     row.FacultyID,
		Faculty:       row.Faculty,
		RatingAverage: ratingAverage(row.RatingSum, row.RatingCount),
		RatingCount:   row.RatingCount,
		IsMine:        row.AuthorID != nil && *row.AuthorID == viewerID,
		MyRating:      row.MyRating,
	}
}
