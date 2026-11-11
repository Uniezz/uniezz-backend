package study

import (
	"context"

	"github.com/google/uuid"
)

// ForgetUser removes the student's files, comments and ratings through the
// same paths a student takes, so rating counters stay right and file uploads
// leave storage. Courses the student created stay, without an author. It must
// run before the user row is deleted: the foreign keys of files and comments
// refuse the delete while anything is left.
func (m Module) ForgetUser(ctx context.Context, userID uuid.UUID) error {
	fileIDs, err := m.files.idsByAuthor(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range fileIDs {
		if err := m.deleteFile(ctx, userID, id); err != nil {
			return err
		}
	}
	if err := m.comments.deleteByAuthor(ctx, userID); err != nil {
		return err
	}

	courseIDs, err := m.courses.ratedBy(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range courseIDs {
		if err := m.removeCourseRating(ctx, userID, id); err != nil {
			return err
		}
	}
	professorIDs, err := m.professors.ratedBy(ctx, userID)
	if err != nil {
		return err
	}
	for _, id := range professorIDs {
		if err := m.removeProfessorRating(ctx, userID, id); err != nil {
			return err
		}
	}
	return nil
}
