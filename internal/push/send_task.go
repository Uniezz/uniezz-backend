package push

import (
	"context"

	"github.com/google/uuid"
)

type Event string

const (
	EventMessage Event = "message"
	EventRequest Event = "request"
	EventMatch   Event = "match"
)

type SendArgs struct {
	Event          Event       `json:"event"`
	RecipientIDs   []uuid.UUID `json:"recipientIds"`
	ActorID        uuid.UUID   `json:"actorId"`
	ConversationID *uuid.UUID  `json:"conversationId,omitempty"`
	GroupName      *string     `json:"groupName,omitempty"`
	Preview        string      `json:"preview,omitempty"`
}

func (SendArgs) Kind() string { return "push.send.v1" }

func (m Module) send(ctx context.Context, args SendArgs) error {
	targets, err := m.tokens.listTargets(ctx, args.RecipientIDs)
	if err != nil || len(targets) == 0 {
		return err
	}
	name, err := m.students.firstName(ctx, args.ActorID)
	if err != nil {
		return err
	}

	messages := make([]expoMessage, 0, len(targets))
	for _, t := range targets {
		messages = append(messages, message(args, name, t))
	}
	invalid, sendErr := m.expo.send(ctx, messages)
	if len(invalid) > 0 {
		if err := m.tokens.deleteMany(ctx, invalid); err != nil {
			return err
		}
	}
	return sendErr
}

func message(args SendArgs, name *string, t targetRow) expoMessage {
	title := fallbackTitle
	if name != nil && *name != "" {
		title = *name
	}
	body := args.Preview
	if body == "" {
		body = photoPreview
	}
	data := map[string]string{"event": string(args.Event)}
	if args.ConversationID != nil {
		data["conversationId"] = args.ConversationID.String()
	}

	switch args.Event {
	case EventMatch:
		body = matchBody(t.Language)
		data["userId"] = args.ActorID.String()
	case EventMessage:
		if args.GroupName != nil {
			body = title + ": " + body
			title = *args.GroupName
		}
	}
	return expoMessage{To: t.Token, Title: title, Body: body, Data: data, Sound: "default", ChannelID: "default"}
}
