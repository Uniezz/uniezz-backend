package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	maxEventTitle = 100
	maxEventPlace = 100
	shownAnswers  = 5
)

type EventAnswer string

const (
	AnswerGoing EventAnswer = "going"
	AnswerMaybe EventAnswer = "maybe"
	AnswerNo    EventAnswer = "no"
)

type eventChange struct {
	Title    string
	StartsAt time.Time
	EndsAt   *time.Time
	Place    *string
}

type eventPayload struct {
	Title      string     `json:"title"`
	StartsAt   time.Time  `json:"startsAt"`
	EndsAt     *time.Time `json:"endsAt"`
	Place      *string    `json:"place"`
	CanceledAt *time.Time `json:"canceledAt"`
}

func (c eventChange) payload(now time.Time) (eventPayload, error) {
	title := text.Plain(c.Title, false)
	if n := utf8.RuneCountInString(title); n == 0 || n > maxEventTitle {
		return eventPayload{}, ErrInvalidInput
	}
	if err := allowedText("title", title); err != nil {
		return eventPayload{}, err
	}
	startsAt := c.StartsAt.UTC()
	if !startsAt.After(now) {
		return eventPayload{}, ErrInvalidInput
	}
	p := eventPayload{Title: title, StartsAt: startsAt}
	if c.EndsAt != nil {
		endsAt := c.EndsAt.UTC()
		if !endsAt.After(startsAt) {
			return eventPayload{}, ErrInvalidInput
		}
		p.EndsAt = &endsAt
	}
	if c.Place != nil {
		place := text.Plain(*c.Place, false)
		if utf8.RuneCountInString(place) > maxEventPlace {
			return eventPayload{}, ErrInvalidInput
		}
		if err := allowedText("place", place); err != nil {
			return eventPayload{}, err
		}
		if place != "" {
			p.Place = &place
		}
	}
	return p, nil
}

func (p eventPayload) ended(now time.Time) bool {
	if p.EndsAt != nil {
		return !p.EndsAt.After(now)
	}
	return !p.StartsAt.After(now)
}

type answerChange struct {
	MessageID uuid.UUID
	Answer    EventAnswer
}

func (m Module) answerEvent(ctx context.Context, viewerID uuid.UUID, c answerChange) (*Message, error) {
	if c.Answer != AnswerGoing && c.Answer != AnswerMaybe && c.Answer != AnswerNo {
		return nil, ErrInvalidInput
	}

	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, _, err := m.lockCard(ctx, viewerID, c.MessageID, MessageEvent)
		if err != nil {
			return err
		}
		var event eventPayload
		if err := json.Unmarshal(row.Payload, &event); err != nil {
			return fmt.Errorf("decode event: %w", err)
		}
		if event.CanceledAt != nil {
			return ErrEventCanceled
		}
		if event.ended(m.now()) {
			return ErrEventEnded
		}
		changed, err := m.events.setAnswer(ctx, c.MessageID, viewerID, c.Answer)
		if err != nil || !changed {
			return err
		}
		notes, err = m.touchMessage(ctx, c.MessageID)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.publish(ctx, notes)
	return m.getMessage(ctx, viewerID, c.MessageID)
}

func (m Module) clearAnswer(ctx context.Context, viewerID, messageID uuid.UUID) error {
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, _, err := m.lockCard(ctx, viewerID, messageID, MessageEvent); err != nil {
			return err
		}
		removed, err := m.events.deleteAnswer(ctx, messageID, viewerID)
		if err != nil || !removed {
			return err
		}
		notes, err = m.touchMessage(ctx, messageID)
		return err
	})
	if err != nil {
		return err
	}
	m.publish(ctx, notes)
	return nil
}

func (m Module) cancelEvent(ctx context.Context, viewerID, messageID uuid.UUID) (*Message, error) {
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, a, err := m.lockCard(ctx, viewerID, messageID, MessageEvent)
		if err != nil {
			return err
		}
		if (row.SenderID == nil || *row.SenderID != viewerID) && a.Role != RoleOwner {
			return ErrForbidden
		}
		canceled, err := m.events.cancel(ctx, messageID)
		if err != nil || !canceled {
			return err
		}
		notes, err = m.touchMessage(ctx, messageID)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.publish(ctx, notes)
	return m.getMessage(ctx, viewerID, messageID)
}

func (m Module) eventViews(ctx context.Context, viewerID uuid.UUID, rows []messageRow, now time.Time) (map[uuid.UUID]*Event, error) {
	events := map[uuid.UUID]*Event{}
	payloads := map[uuid.UUID]eventPayload{}
	var ids []uuid.UUID
	for _, row := range rows {
		if row.Kind != MessageEvent || row.DeletedAt != nil {
			continue
		}
		var p eventPayload
		if err := json.Unmarshal(row.Payload, &p); err != nil {
			return nil, fmt.Errorf("decode event: %w", err)
		}
		payloads[row.ID] = p
		ids = append(ids, row.ID)
	}
	if len(ids) == 0 {
		return events, nil
	}

	counts, err := m.events.counts(ctx, ids)
	if err != nil {
		return nil, err
	}
	mine := map[uuid.UUID]EventAnswer{}
	attendeeIDs := map[uuid.UUID][]uuid.UUID{}
	if viewerID != uuid.Nil {
		if mine, err = m.events.answersOf(ctx, ids, viewerID); err != nil {
			return nil, err
		}
		if attendeeIDs, err = m.events.recentAttendees(ctx, ids, viewerID, shownAnswers); err != nil {
			return nil, err
		}
	}
	var everyone []uuid.UUID
	for _, list := range attendeeIDs {
		everyone = append(everyone, list...)
	}
	students, err := m.studentMap(ctx, everyone)
	if err != nil {
		return nil, err
	}

	for id, p := range payloads {
		event := &Event{
			Title:      p.Title,
			StartsAt:   p.StartsAt,
			EndsAt:     p.EndsAt,
			Place:      p.Place,
			CanceledAt: p.CanceledAt,
			Ended:      p.ended(now),
			Going:      counts[id][AnswerGoing],
			Maybe:      counts[id][AnswerMaybe],
			No:         counts[id][AnswerNo],
			Attendees:  make([]Student, 0, len(attendeeIDs[id])),
		}
		if answer, ok := mine[id]; ok {
			event.MyAnswer = &answer
		}
		for _, attendee := range attendeeIDs[id] {
			event.Attendees = append(event.Attendees, studentOf(students, &attendee))
		}
		events[id] = event
	}
	return events, nil
}
