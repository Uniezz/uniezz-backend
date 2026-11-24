// Package moderation takes reports about posts, comments, students and chat messages. Each
// report keeps a copy of the reported content, so review in a later sprint
// does not depend on the content still existing.
package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/chat"
	"github.com/Uniezz/uniezz-backend/internal/connections"
	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/feed"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/study"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	reportLimit           = 5
	autoHideReports       = 3
	reportWindow          = time.Hour
	anonymiseReportsEvery = 24 * time.Hour
	pruneActionsEvery     = 24 * time.Hour
)

var (
	_ feedTargets      = feed.Module{}
	_ profileTargets   = profile.Module{}
	_ candidateTargets = connections.Module{}
	_ studyTargets     = study.Module{}
	_ chatTargets      = chat.Module{}
	_ staffService     = auth.Module{}
)

type feedTargets interface {
	ReportTarget(ctx context.Context, targetType string, id uuid.UUID) (uuid.UUID, map[string]any, error)
	RemovePost(ctx context.Context, postID uuid.UUID) error
	RemoveComment(ctx context.Context, commentID uuid.UUID) error
	HideReported(ctx context.Context, targetType string, id uuid.UUID) error
	RestoreReported(ctx context.Context, targetType string, id uuid.UUID) error
}

type profileTargets interface {
	ReportTarget(ctx context.Context, userID uuid.UUID) (map[string]any, error)
}

type candidateTargets interface {
	ReportTarget(ctx context.Context, viewerID, candidateID uuid.UUID) (uuid.UUID, map[string]any, error)
	HideCandidate(ctx context.Context, viewerID, userID uuid.UUID) error
}

type studyTargets interface {
	ReportTarget(ctx context.Context, targetType string, id uuid.UUID) (uuid.UUID, map[string]any, error)
	RemoveFile(ctx context.Context, id uuid.UUID) error
	RemoveCourse(ctx context.Context, id uuid.UUID) error
	RemoveCourseComment(ctx context.Context, id uuid.UUID) error
	RemoveProfessorReview(ctx context.Context, id uuid.UUID) error
}

type chatTargets interface {
	ReportTarget(ctx context.Context, reporterID, messageID uuid.UUID) (uuid.UUID, map[string]any, error)
}

type staffService interface {
	GrantRole(ctx context.Context, userID uuid.UUID, role auth.Role) error
	RevokeRole(ctx context.Context, userID uuid.UUID, role auth.Role) error
	Ban(ctx context.Context, userID uuid.UUID) error
	Unban(ctx context.Context, userID uuid.UUID) error
	BannedAt(ctx context.Context, userID uuid.UUID) (*time.Time, error)
}

type limiter interface {
	Allow(ctx context.Context, key string) error
}

type Module struct {
	tx         database.Transactor
	reports    *reportRepository
	queue      *queueRepository
	feed       feedTargets
	profiles   profileTargets
	candidates candidateTargets
	study      studyTargets
	chat       chatTargets
	staff      staffService
	moderators *moderatorRepository
	actions    *actionRepository
	limiter    limiter
}

func New(pool *pgxpool.Pool, feedModule feedTargets, profileModule profileTargets, candidates candidateTargets, studyModule studyTargets, chatModule chatTargets, staff staffService, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{
		tx:         db,
		reports:    newReportRepository(db),
		queue:      newQueueRepository(db),
		feed:       feedModule,
		profiles:   profileModule,
		candidates: candidates,
		study:      studyModule,
		chat:       chatModule,
		staff:      staff,
		moderators: newModeratorRepository(db),
		actions:    newActionRepository(db),
		limiter:    ratelimit.New(pool, "report", reportLimit, reportWindow),
	}

	if err := jobs.Register(runner, m.anonymiseReports, jobs.Options{Timeout: time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("moderation: %w", err)
	}
	if err := jobs.Schedule(runner, "moderation-anonymise-reports", AnonymiseReportsArgs{}, anonymiseReportsEvery); err != nil {
		return Module{}, fmt.Errorf("moderation: %w", err)
	}
	if err := jobs.Register(runner, m.pruneActions, jobs.Options{Timeout: time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("moderation: %w", err)
	}
	if err := jobs.Schedule(runner, "moderation-prune-actions", PruneActionsArgs{}, pruneActionsEvery); err != nil {
		return Module{}, fmt.Errorf("moderation: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	reports := &reportHandler{moderation: m}
	huma.Register(api, createReportOperation, reports.createReport)

	queue := &queueHandler{moderation: m}
	huma.Register(api, listQueueOperation, queue.listQueue)
	huma.Register(api, getReportOperation, queue.getReport)
	huma.Register(api, updateReportOperation, queue.updateReport)

	removals := &removalHandler{moderation: m}
	huma.Register(api, removePostOperation, removals.removePost)
	huma.Register(api, removeCommentOperation, removals.removeComment)
	huma.Register(api, removeFileOperation, removals.removeFile)
	huma.Register(api, removeCourseOperation, removals.removeCourse)
	huma.Register(api, removeCourseCommentOperation, removals.removeCourseComment)
	huma.Register(api, removeProfessorReviewOperation, removals.removeProfessorReview)

	moderators := &moderatorHandler{moderation: m}
	huma.Register(api, listModeratorsOperation, moderators.listModerators)
	huma.Register(api, grantModeratorOperation, moderators.grantModerator)
	huma.Register(api, revokeModeratorOperation, moderators.revokeModerator)

	bans := &banHandler{moderation: m}
	huma.Register(api, getBanOperation, bans.getBan)
	huma.Register(api, banUserOperation, bans.banUser)
	huma.Register(api, unbanUserOperation, bans.unbanUser)

	actions := &actionHandler{moderation: m}
	huma.Register(api, listActionsOperation, actions.listActions)
}
