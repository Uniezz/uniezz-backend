package rewards

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

const (
	questZone   = "Europe/Warsaw"
	likesTarget = 3
)

type Quest string

const (
	QuestLike  Quest = "like"
	QuestReply Quest = "reply"
	QuestPost  Quest = "post"
)

// Progress records one quest action for today in Warsaw time and pays the daily reward once all
// quests are done. Call it inside the transaction of the action.
func (m Module) Progress(ctx context.Context, userID uuid.UUID, quest Quest) error {
	var step questStep
	switch quest {
	case QuestLike:
		step.Likes = 1
	case QuestReply:
		step.Replied = true
	case QuestPost:
		step.Posted = true
	default:
		return fmt.Errorf("record quest: unknown quest %q", quest)
	}

	return m.tx.WithTx(ctx, func(ctx context.Context) error {
		day, err := m.quests.record(ctx, userID, questZone, step)
		if err != nil {
			return err
		}
		if day.Likes < likesTarget || !day.Replied || !day.Posted {
			return nil
		}
		completed, err := m.quests.complete(ctx, day.ID)
		if err != nil || !completed {
			return err
		}
		_, err = m.Grant(ctx, userID, ReasonDailyQuests, &day.ID, nil)
		return err
	})
}
