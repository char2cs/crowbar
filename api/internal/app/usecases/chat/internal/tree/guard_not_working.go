package tree

import (
	"context"
	"errors"
	"fmt"
)

var ErrSubtreeWorking = errors.New("tree: a working chat refuses this verb")

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
			return ErrSubtreeWorking
		}
	}
	return nil
}
