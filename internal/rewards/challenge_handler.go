package rewards

import (
	"context"
	"net/http"
	"time"

	"github.com/Uniezz/uniezz-backend/internal/auth"
	"github.com/Uniezz/uniezz-backend/internal/httpapi"
	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
)

type challengeHandler struct {
	rewards Module
}

const actionDoc = "like_given: liking another student's post. like_received: another student liked your post. " +
	"comment_posted: the first comment on another student's post. reply_posted: replying to another student's comment. " +
	"post_created: creating a post. file_uploaded: adding an exam file. course_rated, professor_rated: rating one. " +
	"connection_made: a new connection. profile_completed: filling every profile field. friend_invited: an invited student signs up. " +
	"faq_approved: a moderator approves your FAQ proposal."

func (Action) TransformSchema(_ huma.Registry, schema *huma.Schema) *huma.Schema {
	schema.Enum = nil
	if schema.Extensions == nil {
		schema.Extensions = map[string]any{}
	}
	schema.Extensions["x-extensible-enum"] = []any{
		ActionLikeGiven, ActionLikeReceived, ActionCommentPosted, ActionReplyPosted, ActionPostCreated, ActionFileUploaded,
		ActionCourseRated, ActionProfessorRated, ActionConnectionMade, ActionProfileCompleted, ActionFriendInvited, ActionFAQApproved,
	}
	return schema
}

type ChallengeText struct {
	Title       string  `json:"title" maxLength:"400" doc:"Up to 80 characters after HTML is stripped" example:"Like 5 posts"`
	Description *string `json:"description,omitempty" maxLength:"2000" doc:"Up to 300 characters after HTML is stripped; left out when empty" example:"Show other students some love this week."`
}

type Challenge struct {
	ID         uuid.UUID                `json:"id" format:"uuid"`
	Action     Action                   `json:"action" doc:"What counts. More values may be added."`
	Target     int                      `json:"target" minimum:"1" example:"5"`
	Period     Period                   `json:"period" enum:"day,week,once" doc:"day and week start over at midnight and on Monday, Europe/Warsaw; once never starts over"`
	Reward     int                      `json:"reward" minimum:"0" doc:"Coins paid when the target is reached in a period" example:"20"`
	Texts      map[string]ChallengeText `json:"texts" doc:"By two-letter language code; pl is always there"`
	StartsAt   *time.Time               `json:"startsAt" nullable:"true" doc:"Null when the challenge has no start date"`
	EndsAt     *time.Time               `json:"endsAt" nullable:"true" doc:"Null when the challenge runs until it is archived"`
	ArchivedAt *time.Time               `json:"archivedAt" nullable:"true" doc:"Null while the challenge is not archived"`
	CreatedAt  time.Time                `json:"createdAt"`
}

type MyChallenge struct {
	ID        uuid.UUID                `json:"id" format:"uuid"`
	Action    Action                   `json:"action" doc:"What counts. More values may be added."`
	Target    int                      `json:"target" minimum:"1" example:"5"`
	Period    Period                   `json:"period" enum:"day,week,once"`
	Reward    int                      `json:"reward" minimum:"0" example:"20"`
	Texts     map[string]ChallengeText `json:"texts" doc:"By two-letter language code; show the student's language, or pl when it is missing"`
	EndsAt    *time.Time               `json:"endsAt" nullable:"true" doc:"Null when the challenge has no end date"`
	Progress  int                      `json:"progress" minimum:"0" example:"2"`
	Completed bool                     `json:"completed" doc:"True once the target is reached in this period and the reward was paid"`
	ResetsAt  *time.Time               `json:"resetsAt" nullable:"true" doc:"When progress starts over; null for a once challenge"`
}

type challengeInput struct {
	ID uuid.UUID `path:"id" format:"uuid"`
}

type challengeOutput struct {
	Body Challenge
}

type listChallengesInput struct {
	httpapi.PageQuery
}

type listChallengesOutput struct {
	Body struct {
		Items      []Challenge `json:"items"`
		NextCursor *string     `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

type listMyChallengesOutput struct {
	Body struct {
		Items      []MyChallenge `json:"items"`
		NextCursor *string       `json:"nextCursor" nullable:"true" doc:"Pass as cursor to get the next page; null on the last page"`
	}
}

type createChallengeInput struct {
	Body struct {
		Action   string                   `json:"action" enum:"like_given,like_received,comment_posted,reply_posted,post_created,file_uploaded,course_rated,professor_rated,connection_made,profile_completed,friend_invited,faq_approved" doc:"What counts. Set once."`
		Target   int                      `json:"target" minimum:"1" maximum:"1000" doc:"Set once" example:"5"`
		Period   Period                   `json:"period" enum:"day,week,once" doc:"Set once"`
		Reward   int                      `json:"reward" minimum:"0" maximum:"1000" example:"20"`
		Texts    map[string]ChallengeText `json:"texts" doc:"By two-letter language code, at most 10; pl is required"`
		StartsAt *time.Time               `json:"startsAt,omitempty"`
		EndsAt   *time.Time               `json:"endsAt,omitempty" doc:"Must be after startsAt"`
	}
}

type updateChallengeInput struct {
	ID   uuid.UUID `path:"id" format:"uuid"`
	Body struct {
		Texts    *map[string]ChallengeText `json:"texts,omitempty" doc:"Replaces every text; pl is required"`
		Reward   *int                      `json:"reward,omitempty" minimum:"0" maximum:"1000"`
		StartsAt *time.Time                `json:"startsAt,omitempty"`
		EndsAt   *time.Time                `json:"endsAt,omitempty" doc:"Must be after startsAt"`
	}
}

var listMyChallengesOperation = auth.Authenticated(huma.Operation{
	OperationID: "listMyChallenges",
	Method:      http.MethodGet,
	Path:        "/rewards/me/challenges",
	Summary:     "List the challenges running now with the logged-in student's progress",
	Description: "Newest first. Progress counts only actions done while the challenge runs, and undoing an action does not take it back.",
	Tags:        []string{"rewards"},
	Errors:      []int{http.StatusBadRequest},
})

func (h *challengeHandler) listMyChallenges(ctx context.Context, in *listChallengesInput) (*listMyChallengesOutput, error) {
	userID, ok := auth.UserIDFrom(ctx)
	if !ok {
		return nil, httpapi.Unauthorized()
	}

	items, next, err := h.rewards.myChallenges(ctx, userID, challengeQuery{Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listMyChallengesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

var listChallengesOperation = auth.Authorized(huma.Operation{
	OperationID: "listChallenges",
	Method:      http.MethodGet,
	Path:        "/rewards/challenges",
	Summary:     "List every challenge, archived ones included",
	Description: "Newest first. Admins only.",
	Tags:        []string{"rewards"},
	Errors:      []int{http.StatusBadRequest},
}, auth.RoleAdmin)

func (h *challengeHandler) listChallenges(ctx context.Context, in *listChallengesInput) (*listChallengesOutput, error) {
	items, next, err := h.rewards.listChallenges(ctx, challengeQuery{Cursor: in.Cursor, Limit: in.Limit})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	out := &listChallengesOutput{}
	out.Body.Items = items
	out.Body.NextCursor = next
	return out, nil
}

var createChallengeOperation = auth.Authorized(huma.Operation{
	OperationID:   "createChallenge",
	Method:        http.MethodPost,
	Path:          "/rewards/challenges",
	Summary:       "Create a challenge",
	Description:   "Students see it while it runs. Action, target and period cannot change later; archive the challenge and create a new one instead. Admins only.",
	Tags:          []string{"rewards"},
	DefaultStatus: http.StatusCreated,
	Errors:        []int{http.StatusBadRequest},
}, auth.RoleAdmin)

func (h *challengeHandler) createChallenge(ctx context.Context, in *createChallengeInput) (*challengeOutput, error) {
	challenge, err := h.rewards.createChallenge(ctx, challengeChange{
		Action:   Action(in.Body.Action),
		Target:   in.Body.Target,
		Period:   in.Body.Period,
		Reward:   in.Body.Reward,
		Texts:    in.Body.Texts,
		StartsAt: in.Body.StartsAt,
		EndsAt:   in.Body.EndsAt,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &challengeOutput{Body: *challenge}, nil
}

var getChallengeOperation = auth.Authorized(huma.Operation{
	OperationID: "getChallenge",
	Method:      http.MethodGet,
	Path:        "/rewards/challenges/{id}",
	Summary:     "Get a challenge",
	Description: "Archived challenges too. Admins only.",
	Tags:        []string{"rewards"},
	Errors:      []int{http.StatusNotFound},
}, auth.RoleAdmin)

func (h *challengeHandler) getChallenge(ctx context.Context, in *challengeInput) (*challengeOutput, error) {
	challenge, err := h.rewards.challenge(ctx, in.ID)
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &challengeOutput{Body: *challenge}, nil
}

var updateChallengeOperation = auth.Authorized(huma.Operation{
	OperationID: "updateChallenge",
	Method:      http.MethodPatch,
	Path:        "/rewards/challenges/{id}",
	Summary:     "Update a challenge's texts, reward or dates",
	Description: "Changes only the fields that are present and returns the challenge. A new reward applies to periods completed after the change. Admins only.",
	Tags:        []string{"rewards"},
	Errors:      []int{http.StatusBadRequest, http.StatusNotFound},
}, auth.RoleAdmin)

func (h *challengeHandler) updateChallenge(ctx context.Context, in *updateChallengeInput) (*challengeOutput, error) {
	challenge, err := h.rewards.updateChallenge(ctx, in.ID, challengeEdit{
		Texts:    in.Body.Texts,
		Reward:   in.Body.Reward,
		StartsAt: in.Body.StartsAt,
		EndsAt:   in.Body.EndsAt,
	})
	if err != nil {
		return nil, errorResponses.Error(err)
	}
	return &challengeOutput{Body: *challenge}, nil
}

var archiveChallengeOperation = auth.Authorized(huma.Operation{
	OperationID:   "archiveChallenge",
	Method:        http.MethodDelete,
	Path:          "/rewards/challenges/{id}",
	Summary:       "Archive a challenge",
	Description:   "Students stop seeing it and their actions stop counting; coins already paid stay. Archiving again succeeds. Admins only.",
	Tags:          []string{"rewards"},
	DefaultStatus: http.StatusNoContent,
	Errors:        []int{http.StatusNotFound},
}, auth.RoleAdmin)

func (h *challengeHandler) archiveChallenge(ctx context.Context, in *challengeInput) (*struct{}, error) {
	if err := h.rewards.archiveChallenge(ctx, in.ID); err != nil {
		return nil, errorResponses.Error(err)
	}
	return nil, nil
}
