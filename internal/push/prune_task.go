package push

import (
	"context"
	"time"
)

const deliveryAge = time.Hour

type PruneArgs struct{}

func (PruneArgs) Kind() string { return "push.prune.v1" }

func (m Module) prune(ctx context.Context, _ PruneArgs) error {
	return m.deliveries.prune(ctx, deliveryAge)
}
