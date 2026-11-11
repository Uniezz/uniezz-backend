package connections

import (
	"context"
	"errors"
)

type PruneDiscoveryArgs struct{}

func (PruneDiscoveryArgs) Kind() string { return "connections.prune-discovery.v1" }

func (m Module) pruneDiscovery(ctx context.Context, _ PruneDiscoveryArgs) error {
	return errors.Join(
		m.discovery.pruneCandidates(ctx, candidateLifetime),
		m.discovery.prunePasses(ctx, passCooldown),
	)
}
