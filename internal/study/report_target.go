package study

import (
	"context"

	"github.com/google/uuid"
)

type reportTarget string

const (
	reportTargetFile            reportTarget = "file"
	reportTargetCourse          reportTarget = "course"
	reportTargetCourseComment   reportTarget = "course_comment"
	reportTargetProfessorReview reportTarget = "professor_review"
)

// ReportTarget returns the author of a reported course, file, course comment
// or professor review and a copy of what was reported, kept with the report.
func (m Module) ReportTarget(ctx context.Context, targetType string, id uuid.UUID) (uuid.UUID, map[string]any, error) {
	switch reportTarget(targetType) {
	case reportTargetCourse:
		row, err := m.courses.get(ctx, uuid.Nil, id)
		if err != nil {
			return uuid.Nil, nil, err
		}
		var authorID uuid.UUID
		if row.AuthorID != nil {
			authorID = *row.AuthorID
		}
		return authorID, map[string]any{"name": row.Name, "description": row.Description}, nil
	case reportTargetFile:
		row, err := m.files.get(ctx, uuid.Nil, id)
		if err != nil {
			return uuid.Nil, nil, err
		}
		return row.AuthorID, map[string]any{"title": row.Title, "type": row.Type, "courseId": row.CourseID}, nil
	case reportTargetCourseComment:
		row, err := m.comments.get(ctx, id)
		if err != nil {
			return uuid.Nil, nil, err
		}
		return row.AuthorID, map[string]any{"content": row.Content, "courseId": row.CourseID}, nil
	case reportTargetProfessorReview:
		row, err := m.reviews.get(ctx, id)
		if err != nil {
			return uuid.Nil, nil, err
		}
		return row.AuthorID, map[string]any{"content": row.Content, "professorId": row.ProfessorID}, nil
	default:
		return uuid.Nil, nil, ErrNotFound
	}
}
