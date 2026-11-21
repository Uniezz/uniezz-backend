package moderation

import (
	"context"
	"time"
)

const actionRetention = 365 * 24 * time.Hour

type PruneActionsArgs struct{}

func (PruneActionsArgs) Kind() string { return "moderation.prune-actions.v1" }

func (m Module) pruneActions(ctx context.Context, _ PruneActionsArgs) error {
	return m.actions.prune(ctx, actionRetention)
}
