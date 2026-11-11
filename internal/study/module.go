// Package study owns what students share about their courses: the course
// catalogue with ratings, files and comments, and the professors imported from
// POL-on that students rate.
package study

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/media"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	createLimit  = 30
	createWindow = time.Hour
)

var _ mediaService = media.Module{}

type mediaService interface {
	CheckUpload(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	MarkReady(ctx context.Context, mediaID, ownerUserID uuid.UUID, kind media.Kind) error
	MarkDeleting(ctx context.Context, mediaID uuid.UUID) error
	Delete(ctx context.Context, mediaID uuid.UUID) error
	URLOf(ctx context.Context, ref media.Ref) (string, error)
}

var _ limiter = (*ratelimit.Limiter)(nil)

type limiter interface {
	Allow(ctx context.Context, key string) error
}

type Module struct {
	tx         database.Transactor
	courses    *courseRepository
	files      *fileRepository
	comments   *commentRepository
	professors *professorRepository
	media      mediaService
	limiter    limiter
	radon      *radonClient
}

func New(pool *pgxpool.Pool, mediaModule mediaService, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{
		tx:         db,
		courses:    newCourseRepository(db),
		files:      newFileRepository(db),
		comments:   newCommentRepository(db),
		professors: newProfessorRepository(db),
		media:      mediaModule,
		limiter:    ratelimit.New(pool, "study-create", createLimit, createWindow),
		radon:      newRadonClient(radonEmployeesURL),
	}

	if err := jobs.Register(runner, m.importProfessors, jobs.Options{Timeout: importTimeout, MaxAttempts: importAttempts}); err != nil {
		return Module{}, fmt.Errorf("study: %w", err)
	}
	for _, institution := range radonInstitutions {
		args := ImportProfessorsArgs{University: institution.university}
		if err := jobs.Schedule(runner, args.scheduleName(), args, importInterval); err != nil {
			return Module{}, fmt.Errorf("study: %w", err)
		}
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	courses := &courseHandler{study: m}
	files := &fileHandler{study: m}
	comments := &commentHandler{study: m}
	professors := &professorHandler{study: m}

	huma.Register(api, createCourseOperation, courses.createCourse)
	huma.Register(api, listCoursesOperation, courses.listCourses)
	huma.Register(api, getCourseOperation, courses.getCourse)
	huma.Register(api, updateCourseOperation, courses.updateCourse)
	huma.Register(api, rateCourseOperation, courses.rateCourse)
	huma.Register(api, addCourseFileOperation, files.addCourseFile)
	huma.Register(api, listCourseFilesOperation, files.listCourseFiles)
	huma.Register(api, searchFilesOperation, files.searchFiles)
	huma.Register(api, getFileOperation, files.getFile)
	huma.Register(api, deleteFileOperation, files.deleteFile)
	huma.Register(api, addCourseCommentOperation, comments.addCourseComment)
	huma.Register(api, listCourseCommentsOperation, comments.listCourseComments)
	huma.Register(api, deleteCommentOperation, comments.deleteComment)
	huma.Register(api, listProfessorsOperation, professors.listProfessors)
	huma.Register(api, getProfessorOperation, professors.getProfessor)
	huma.Register(api, rateProfessorOperation, professors.rateProfessor)
}
