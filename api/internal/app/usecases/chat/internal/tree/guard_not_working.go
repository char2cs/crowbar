package tree

import (
	"context"
	"fmt"

)

// Busy answers whether a chat is working.
type Busy interface {
	ChatBusy(ctx context.Context, chatID string) (bool, error)
}

func guardNotWorking(
	ctx context.Context,
	subtreeIDs []string,
	busy Busy,
) error {
	for _, id := range subtreeIDs {
		working, err := busy.ChatBusy(ctx, id)
		if err != nil {
			return fmt.Errorf("tree: is %s working: %w", id, err)
		}
		if working {
			return fmt.Errorf("tree: %s is working: %w", id, ErrSubtreeWorking)
		}
	}
	return nil
}
