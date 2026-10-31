package feed

import (
	"context"
	"time"
)

const requestKeyLifetime = 24 * time.Hour

type PruneKeysArgs struct{}

func (PruneKeysArgs) Kind() string { return "feed.prune-request-keys.v1" }

func (m Module) pruneKeys(ctx context.Context, _ PruneKeysArgs) error {
	return m.keys.pruneOlderThan(ctx, requestKeyLifetime)
}
