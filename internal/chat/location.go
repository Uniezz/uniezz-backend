package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
	"unicode/utf8"

	"github.com/Uniezz/uniezz-backend/internal/text"
	"github.com/google/uuid"
)

const (
	maxLocationLabel  = 100
	liveFor           = time.Hour
	liveUpdateEvery   = 10 * time.Second
	liveFinalizeBatch = 200
	maxAccuracy       = 1500
)

type locationChange struct {
	Lat   float64
	Lng   float64
	Label *string
	Live  bool
}

type livePayload struct {
	Until     time.Time  `json:"until"`
	StoppedAt *time.Time `json:"stoppedAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type locationPayload struct {
	Lat   float64      `json:"lat"`
	Lng   float64      `json:"lng"`
	Label *string      `json:"label"`
	Live  *livePayload `json:"live"`
}

func roundCoordinate(v float64) float64 {
	return math.Round(v*1e6) / 1e6
}

func validCoordinates(lat, lng float64) bool {
	return lat >= -90 && lat <= 90 && lng >= -180 && lng <= 180 && !math.IsNaN(lat) && !math.IsNaN(lng)
}

func (c locationChange) payload() (locationPayload, error) {
	if !validCoordinates(c.Lat, c.Lng) {
		return locationPayload{}, ErrInvalidInput
	}
	p := locationPayload{Lat: roundCoordinate(c.Lat), Lng: roundCoordinate(c.Lng)}
	if c.Label != nil {
		label := text.Plain(*c.Label, false)
		if utf8.RuneCountInString(label) > maxLocationLabel {
			return locationPayload{}, ErrInvalidInput
		}
		if err := allowedText("label", label); err != nil {
			return locationPayload{}, err
		}
		if label != "" {
			p.Label = &label
		}
	}
	if c.Live {
		p.Live = &livePayload{}
	}
	return p, nil
}

func (p locationPayload) body() string {
	if p.Label != nil {
		return *p.Label
	}
	return "Location"
}

func (m Module) startLive(ctx context.Context, in *sendInput) error {
	if in.Kind != MessageLocation {
		return nil
	}
	var p locationPayload
	if err := json.Unmarshal(in.Payload, &p); err != nil {
		return fmt.Errorf("decode location: %w", err)
	}
	if p.Live == nil {
		return nil
	}
	now, err := m.lives.now(ctx)
	if err != nil {
		return err
	}
	p.Live = &livePayload{Until: now.Add(liveFor), UpdatedAt: now}
	raw, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode location: %w", err)
	}
	in.Payload = raw
	return nil
}

func (m Module) storeLive(ctx context.Context, senderID uuid.UUID, created *messageRow) error {
	if created.Kind != MessageLocation {
		return nil
	}
	var p locationPayload
	if err := json.Unmarshal(created.Payload, &p); err != nil {
		return fmt.Errorf("decode location: %w", err)
	}
	if p.Live == nil {
		return nil
	}
	return m.lives.create(ctx, liveFields{MessageID: created.ID, UserID: senderID, Lat: p.Lat, Lng: p.Lng, Until: p.Live.Until})
}

type positionChange struct {
	MessageID uuid.UUID
	Lat       float64
	Lng       float64
	Accuracy  *int
	Heading   *int
}

func (c positionChange) fields(userID uuid.UUID) (liveFields, error) {
	if !validCoordinates(c.Lat, c.Lng) {
		return liveFields{}, ErrInvalidInput
	}
	if c.Accuracy != nil && (*c.Accuracy < 0 || *c.Accuracy > maxAccuracy) {
		return liveFields{}, ErrInvalidInput
	}
	if c.Heading != nil && (*c.Heading < 1 || *c.Heading > 360) {
		return liveFields{}, ErrInvalidInput
	}
	return liveFields{MessageID: c.MessageID, UserID: userID, Lat: roundCoordinate(c.Lat), Lng: roundCoordinate(c.Lng), Accuracy: c.Accuracy, Heading: c.Heading}, nil
}

func (m Module) updateLocation(ctx context.Context, viewerID uuid.UUID, c positionChange) error {
	f, err := c.fields(viewerID)
	if err != nil {
		return err
	}
	message, err := m.messages.getVisible(ctx, viewerID, c.MessageID)
	if err != nil {
		return err
	}
	if message.Kind != MessageLocation || message.DeletedAt != nil {
		return ErrNotFound
	}

	updated, err := m.lives.move(ctx, f, liveUpdateEvery)
	if errors.Is(err, ErrNotFound) {
		return m.whyNotMoved(ctx, viewerID, c.MessageID)
	}
	if err != nil {
		return err
	}

	recipients, err := m.members.recipients(ctx, message.ConversationID, viewerID)
	if err != nil {
		return err
	}
	m.hub.publish(recipients, encodeEvent(eventLocationUpdated, locationEvent{
		ConversationID: message.ConversationID,
		MessageID:      c.MessageID,
		Lat:            updated.Lat,
		Lng:            updated.Lng,
		Accuracy:       updated.Accuracy,
		Heading:        updated.Heading,
		UpdatedAt:      updated.UpdatedAt,
	}))
	return nil
}

func (m Module) whyNotMoved(ctx context.Context, viewerID, messageID uuid.UUID) error {
	live, err := m.lives.get(ctx, messageID)
	if errors.Is(err, ErrNotFound) {
		return ErrLocationNotLive
	}
	if err != nil {
		return err
	}
	switch {
	case live.UserID != viewerID:
		return ErrForbidden
	case live.Expired:
		return ErrLocationNotLive
	}
	return ErrRateLimited
}

func (m Module) stopLocation(ctx context.Context, viewerID, messageID uuid.UUID) (*Message, error) {
	var notes []notification
	err := m.tx.WithTx(ctx, func(ctx context.Context) error {
		row, _, err := m.lockCard(ctx, viewerID, messageID, MessageLocation)
		if err != nil {
			return err
		}
		if row.SenderID == nil || *row.SenderID != viewerID {
			return ErrForbidden
		}
		notes, err = m.finishLive(ctx, messageID)
		return err
	})
	if err != nil {
		return nil, err
	}
	m.publish(ctx, notes)
	return m.getMessage(ctx, viewerID, messageID)
}

func (m Module) finishLive(ctx context.Context, messageID uuid.UUID) ([]notification, error) {
	finished, err := m.lives.finish(ctx, messageID)
	if err != nil || !finished {
		return nil, err
	}
	return m.touchMessage(ctx, messageID)
}

func (m Module) expireLive(ctx context.Context, messageID uuid.UUID) ([]notification, error) {
	row, err := m.messages.get(ctx, messageID)
	if err != nil {
		return nil, err
	}
	var notes []notification
	err = m.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, err := m.conversations.lock(ctx, row.ConversationID); err != nil {
			return err
		}
		notes, err = m.finishLive(ctx, messageID)
		return err
	})
	return notes, err
}

func (m Module) locationViews(ctx context.Context, rows []messageRow) (map[uuid.UUID]*Location, error) {
	locations := map[uuid.UUID]*Location{}
	var running []uuid.UUID
	for _, row := range rows {
		if row.Kind != MessageLocation || row.DeletedAt != nil {
			continue
		}
		var p locationPayload
		if err := json.Unmarshal(row.Payload, &p); err != nil {
			return nil, fmt.Errorf("decode location: %w", err)
		}
		location := &Location{Lat: p.Lat, Lng: p.Lng, Label: p.Label}
		if p.Live != nil {
			location.Live = &LiveLocation{Until: p.Live.Until, StoppedAt: p.Live.StoppedAt, UpdatedAt: p.Live.UpdatedAt}
			if p.Live.StoppedAt == nil {
				running = append(running, row.ID)
			}
		}
		locations[row.ID] = location
	}
	if len(running) == 0 {
		return locations, nil
	}

	lives, err := m.lives.getMany(ctx, running)
	if err != nil {
		return nil, err
	}
	for _, live := range lives {
		location := locations[live.MessageID]
		location.Lat = live.Lat
		location.Lng = live.Lng
		location.Live.UpdatedAt = live.UpdatedAt
		location.Live.Accuracy = live.Accuracy
		location.Live.Heading = live.Heading
	}
	return locations, nil
}
