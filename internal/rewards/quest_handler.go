package rewards

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
)

type questHandler struct {
	rewards Module
}

// TransformSchema lists quests as an extensible enum, so adding a quest is not a breaking change and
// clients must ignore a quest they do not know.
func (Quest) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	schema.Enum = nil
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	schema.Extensions["x-extensible-enum"] = []any{"like", "reply", "post"}
	return schema
}

type QuestProgress struct {
	Quest    Quest `json:"quest" doc:"like: like other students' posts; reply: reply to another student's comment; post: create a post"`
	Progress int   `json:"progress" minimum:"0" example:"1"`
	Target   int   `json:"target" minimum:"1" example:"3"`
	Done     bool  `json:"done"`
}

type DailyQuests struct {
	Day       string          `json:"day" format:"date" doc:"Today in Europe/Warsaw" example:"2026-11-15"`
	Quests    []QuestProgress `json:"quests"`
	Reward    int             `json:"reward" doc:"Coins paid once every quest of the day is done" example:"10"`
	Completed bool            `json:"completed" doc:"True once every quest is done today and the reward was paid"`
	ResetsAt  time.Time       `json:"resetsAt" doc:"Next midnight in Europe/Warsaw, when progress starts over"`
}

type getMyDailyQuestsOutput struct {
	Body DailyQuests
}

var getMyDailyQuestsOperation = auth.Protected(huma.Operation{
	OperationID: "getMyDailyQuests",
	Method:      http.MethodGet,
	Path:        "/rewards/me/quests",
	Summary:     "Get the logged-in student's quests for today",
	Description: "Progress resets at midnight Europe/Warsaw. Finishing every quest pays the reward once a day; undoing an action later does not take progress back.",
	Tags:        []string{"rewards"},
})

func (h *questHandler) getMyDailyQuests(ctx context.Context, _ *struct{}) (*getMyDailyQuestsOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	quests, err := h.rewards.dailyQuests(ctx, userID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &getMyDailyQuestsOutput{Body: *quests}, nil
}
