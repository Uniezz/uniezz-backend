package feed

import "context"

type PruneWeeklyLikesArgs struct{}

func (PruneWeeklyLikesArgs) Kind() string { return "feed.prune-weekly-likes.v1" }

func (m Module) pruneWeeklyLikes(ctx context.Context, _ PruneWeeklyLikesArgs) error {
	return m.likes.pruneWeeklyBeforeThisWeek(ctx)
}
