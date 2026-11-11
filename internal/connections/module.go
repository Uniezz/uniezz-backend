package connections

import (
	"context"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/database"
	"github.com/Uniezz/uniezz-backend/internal/profile"
	"github.com/Uniezz/uniezz-backend/internal/ratelimit"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	likeLimit  = 50
	likeWindow = 24 * time.Hour
)

var _ profiles = profile.Module{}

type profiles interface {
	Summaries(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]profile.ProfileSummary, error)
}

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
	likes       limiter
}

func New(pool *pgxpool.Pool, profileModule profiles) Module {
	db := database.New(pool)
	return Module{
		tx:          db,
		preferences: newPreferencesRepository(db),
		discovery:   newDiscoveryRepository(db),
		connections: newConnectionRepository(db),
		blocks:      newBlockRepository(db),
		profiles:    profileModule,
		likes:       ratelimit.New(pool, "connection-like", likeLimit, likeWindow),
	}
}

func (m Module) Register(api huma.API) {
	preferences := &preferencesHandler{connections: m}
	discovery := &discoveryHandler{connections: m}
	connections := &connectionHandler{connections: m}
	blocks := &blockHandler{connections: m}

	huma.Register(api, savePreferencesOperation, preferences.savePreferences)
	huma.Register(api, discoverOperation, discovery.discover)
	huma.Register(api, actOnCandidateOperation, discovery.actOnCandidate)
	huma.Register(api, listConnectionsOperation, connections.listConnections)
	huma.Register(api, deleteConnectionOperation, connections.deleteConnection)
	huma.Register(api, blockStudentOperation, blocks.blockStudent)
	huma.Register(api, unblockStudentOperation, blocks.unblockStudent)
}
