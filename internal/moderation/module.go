// Package moderation takes reports about posts, comments and students. Each
// report keeps a copy of the reported content, so review in a later sprint
// does not depend on the content still existing.
package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/feed"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	reportLimit  = 5
	reportWindow = time.Hour
)

var (
	_ feedTargets    = feed.Module{}
	_ profileTargets = profile.Module{}
)

type feedTargets interface {
	ReportTarget(ctx context.Context, targetType string, id uuid.UUID) (uuid.UUID, map[string]any, error)
}

type profileTargets interface {
	ReportTarget(ctx context.Context, userID uuid.UUID) (map[string]any, error)
}

type limiter interface {
	Allow(ctx context.Context, key string) error
}

type Module struct {
	tx       database.Transactor
	reports  *reportRepository
	feed     feedTargets
	profiles profileTargets
	limiter  limiter
}

func New(pool *pgxpool.Pool, feedModule feedTargets, profileModule profileTargets, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{
		tx:       db,
		reports:  newReportRepository(db),
		feed:     feedModule,
		profiles: profileModule,
		limiter:  ratelimit.New(pool, "report", reportLimit, reportWindow),
	}

	if err := jobs.Register(runner, m.anonymiseReports, jobs.Options{Timeout: time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("moderation: %w", err)
	}
	if err := jobs.Schedule(runner, AnonymiseReportsArgs{}, 24*time.Hour); err != nil {
		return Module{}, fmt.Errorf("moderation: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	reports := &reportHandler{moderation: m}
	huma.Register(api, createReportOperation, reports.createReport)
}
