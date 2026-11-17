package rewards

import (
	"context"
	"fmt"
	"time"

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

// Progress records one quest action for today in Warsaw time, pays the daily reward once all quests
// are done, and returns the coins it paid. Call it inside the transaction of the action.
func (m Module) Progress(ctx context.Context, userID uuid.UUID, quest Quest) (int, error) {
	var step questStep
	switch quest {
	case QuestLike:
		step.Likes = 1
	case QuestReply:
		step.Replied = true
	case QuestPost:
		step.Posted = true
	default:
		return 0, fmt.Errorf("record quest: unknown quest %q", quest)
	}

	paid := 0
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
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
		paid, err = m.Grant(ctx, userID, ReasonDailyQuests, &day.ID, nil)
		return err
	})
	return paid, err
}

func (m Module) dailyQuests(ctx context.Context, userID uuid.UUID) (*DailyQuests, error) {
	day, resetsAt, err := m.quests.today(ctx, userID, questZone)
	if err != nil {
		return nil, err
	}
	return &DailyQuests{
		Day: day.Day.Format(time.DateOnly),
		Quests: []QuestProgress{
			{Quest: QuestLike, Progress: min(day.Likes, likesTarget), Target: likesTarget, Done: day.Likes >= likesTarget},
			{Quest: QuestReply, Progress: count(day.Replied), Target: 1, Done: day.Replied},
			{Quest: QuestPost, Progress: count(day.Posted), Target: 1, Done: day.Posted},
		},
		Reward:    amounts[ReasonDailyQuests],
		Completed: day.CompletedAt != nil,
		ResetsAt:  resetsAt,
	}, nil
}

func count(done bool) int {
	if done {
		return 1
	}
	return 0
}
