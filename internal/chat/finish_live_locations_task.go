package chat

import "context"

type FinishLiveLocationsArgs struct{}

func (FinishLiveLocationsArgs) Kind() string { return "chat.finish-live-locations.v1" }

func (m Module) finishLiveLocations(ctx context.Context, _ FinishLiveLocationsArgs) error {
	ids, err := m.lives.expired(ctx, liveFinalizeBatch)
	if err != nil {
		return err
	}
	for _, id := range ids {
		notes, err := m.expireLive(ctx, id)
		if err != nil {
			return err
		}
		m.publish(ctx, notes)
	}
	return nil
}
