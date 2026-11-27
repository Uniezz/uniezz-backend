package push

import (
	"context"
	"time"
)

const collapseWindow = 30 * time.Second

func (m Module) Notify(ctx context.Context, args SendArgs) error {
	args.Preview = shortPreview(args.Preview)
	if args.Event == EventMessage && args.ConversationID != nil {
		allowed, err := m.deliveries.claim(ctx, args.RecipientIDs, *args.ConversationID, collapseWindow)
		if err != nil {
			return err
		}
		args.RecipientIDs = allowed
	}
	if len(args.RecipientIDs) == 0 {
		return nil
	}
	return m.runner.Enqueue(ctx, args)
}

func (m Module) Wake() {
	m.runner.Wake()
}
