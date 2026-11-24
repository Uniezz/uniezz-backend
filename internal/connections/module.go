// Package connections owns how students meet: anonymous discovery cards,
// likes and passes, matches, and blocks between students.
package connections

import (
	"context"
	"fmt"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/jobs"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/Uniezz/uniezz-backend/internal/rewards"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	likeLimit           = 50
	likeWindow          = 24 * time.Hour
	pruneDiscoveryEvery = 24 * time.Hour
)

var _ profiles = profile.Module{}

type profiles interface {
	Summaries(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]profile.ProfileSummary, error)
	Photos(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID][]string, error)
}

var _ rewardService = rewards.Module{}

type rewardService interface {
	Grant(ctx context.Context, userID uuid.UUID, reason rewards.Reason, refID, actorID *uuid.UUID) (int, error)
	Track(ctx context.Context, userID uuid.UUID, action rewards.Action) (int, error)
}

var _ limiter = (*ratelimit.Limiter)(nil)

type limiter interface {
	Allow(ctx context.Context, key string) error
}

type Module struct {
	tx          database.Transactor
	preferences *preferencesRepository
	discovery   *discoveryRepository
	connections *connectionRepository
	blocks      *blockRepository
	profiles    profiles
	rewards     rewardService
	likes       limiter
	now         func() time.Time
}

func New(pool *pgxpool.Pool, profileModule profiles, rewardsModule rewardService, runner *jobs.Runner) (Module, error) {
	db := database.New(pool)
	m := Module{
		tx:          db,
		preferences: newPreferencesRepository(db),
		discovery:   newDiscoveryRepository(db),
		connections: newConnectionRepository(db),
		blocks:      newBlockRepository(db),
		profiles:    profileModule,
		rewards:     rewardsModule,
		likes:       ratelimit.New(pool, "connection-like", likeLimit, likeWindow),
		now:         time.Now,
	}

	if err := jobs.Register(runner, m.pruneDiscovery, jobs.Options{Timeout: 5 * time.Minute, MaxAttempts: 3}); err != nil {
		return Module{}, fmt.Errorf("connections: %w", err)
	}
	if err := jobs.Schedule(runner, "connections-prune-discovery", PruneDiscoveryArgs{}, pruneDiscoveryEvery); err != nil {
		return Module{}, fmt.Errorf("connections: %w", err)
	}
	return m, nil
}

func (m Module) Register(api huma.API) {
	preferences := &preferencesHandler{connections: m}
	discovery := &discoveryHandler{connections: m}
	connections := &connectionHandler{connections: m}
	blocks := &blockHandler{connections: m}

	huma.Register(api, getPreferencesOperation, preferences.getPreferences)
	huma.Register(api, savePreferencesOperation, preferences.savePreferences)
	huma.Register(api, discoverOperation, discovery.discover)
	huma.Register(api, actOnCandidateOperation, discovery.actOnCandidate)
	huma.Register(api, listConnectionsOperation, connections.listConnections)
	huma.Register(api, deleteConnectionOperation, connections.deleteConnection)
	huma.Register(api, blockStudentOperation, blocks.blockStudent)
	huma.Register(api, unblockStudentOperation, blocks.unblockStudent)
}
