package rewards

import "context"

const questRetentionDays = 30

type PruneQuestsArgs struct{}

func (PruneQuestsArgs) Kind() string { return "rewards.prune-quests.v1" }

func (m Module) pruneQuests(ctx context.Context, _ PruneQuestsArgs) error {
	return m.quests.deleteOlderThan(ctx, questZone, questRetentionDays)
}
