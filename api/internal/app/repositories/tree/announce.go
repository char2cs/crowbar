package tree

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// ChatMoved hears a chat's new placement; kind is the chat frame kind it rides
// ("placement_set" when the parent changed, else "order_set").
type ChatMoved func(ctx context.Context, chatID string, p tree.Placement, kind string)

// Announce tells live clients what each Tree write moved, as the chat
// aggregate's own events did before placement left it: chatMoved hears every
// chat whose placement changed.
func Announce(
	ax asynx.Asynx[tree.Tree],
	chatMoved ChatMoved,
) error {
	if _, err := ax.Subscribe(asynx.Topic("tree.*"), func(ctx context.Context, evt asynxModels.Event[tree.Tree]) {
		announce(ctx, evt.PreviousAggregate, evt.Aggregate, chatMoved)
	}); err != nil {
		return fmt.Errorf("tree: announce: %w", err)
	}
	return nil
}

func announce(
	ctx context.Context,
	before tree.Tree,
	after tree.Tree,
	chatMoved ChatMoved,
) {
	for id, p := range after.Chats {
		was, ok := before.Chats[id]
		switch {
		case !ok || was.ParentID != p.ParentID:
			chatMoved(ctx, id, p, "placement_set")
		case was.Order != p.Order:
			chatMoved(ctx, id, p, "order_set")
		}
	}
}
