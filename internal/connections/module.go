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

type profiles interface {
	ShownProfile(ctx context.Context, viewerID, userID uuid.UUID) (*profile.PublicProfile, error)
}

type Module struct {
	db       database.DB
	tx       database.Transactor
	repo     *repository
	profiles profiles
	likes    limiter
}

type limiter interface {
	Allow(ctx context.Context, key string) error
}

func New(pool *pgxpool.Pool, profileModule profiles) Module {
	db := database.New(pool)
	return Module{
		db:       db,
		tx:       db,
		repo:     newRepository(db),
		profiles: profileModule,
		likes:    ratelimit.New(pool, "connection-like", 50, 24*time.Hour),
	}
}

func (m Module) Register(api huma.API) {
	h := &handler{connections: m}
	huma.Register(api, savePreferencesOperation, h.savePreferences)
	huma.Register(api, discoverOperation, h.discover)
	huma.Register(api, actOperation, h.act)
	huma.Register(api, listConnectionsOperation, h.listConnections)
	huma.Register(api, deleteConnectionOperation, h.deleteConnection)
	huma.Register(api, blockOperation, h.block)
	huma.Register(api, unblockOperation, h.unblock)
}
