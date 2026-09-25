package conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// RestateProvidersFromHistory makes domain.Chat.ProviderID agree with the
// runner history for every chat, once, when an install is upgraded.
//
// The pre-audit daemon answered "as whom does this chat come back" from that
// history first and read the stored field only when there was none, so a row
// it wrote may carry an empty or stale field. Every chat whose history names a
// provider other than the stored one is restated to it; after this the field is
// the one owner and nothing re-derives it. Best-effort per row: it reports
// whether every row was examined, so an incomplete run is repeated next boot.
func (c *Conversations) RestateProvidersFromHistory(ctx context.Context) bool {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "agent: restate chat providers: list", "err", err)
		return false
	}
	complete := true
	for _, row := range rows {
		if row.Type == domain.ChatTypeFolder {
			continue
		}
		if err := c.restateProvider(ctx, row); err != nil {
			slog.WarnContext(ctx, "agent: restate chat provider (best-effort, continuing)",
				"chat_id", row.ID, "err", err)
			complete = false
		}
	}
	return complete
}

func (c *Conversations) restateProvider(
	ctx context.Context,
	row domain.Chat,
) error {
	providerID, found, err := c.newestPlacedProvider(ctx, row.ID)
	if err != nil || !found || providerID == row.ProviderID {
		return err
	}
	if _, err := c.chats.SetProvider(ctx, row.ID, providerID); err != nil &&
		!errors.Is(err, asynxModels.ErrValidation) {
		return fmt.Errorf("set provider %q: %w", providerID, err)
	}
	return nil
}

// newestPlacedProvider is the one-time migration of the old three-projection
// scan: whichever of the chat's conversations, provider-switch interruptions
// and runner placements is NEWEST names the provider last running here.
func (c *Conversations) newestPlacedProvider(
	ctx context.Context,
	chatID string,
) (providerID string, found bool, err error) {
	convs, err := c.runnerStore.ConversationsForChat(ctx, chatID)
	if err != nil {
		return "", false, fmt.Errorf("conversations: %w", err)
	}
	interruptions, err := c.activity.Interruptions(ctx, chatID)
	if err != nil {
		return "", false, fmt.Errorf("interruptions: %w", err)
	}
	placements, err := c.runnerStore.PlacementsForChat(ctx, chatID)
	if err != nil {
		return "", false, fmt.Errorf("placements: %w", err)
	}

	var newest time.Time
	consider := func(id string, at time.Time) {
		if !found || at.After(newest) {
			providerID, newest, found = id, at, true
		}
	}
	for _, conv := range convs {
		consider(conv.ProviderID, conv.LastActiveAt)
	}
	for _, in := range interruptions {
		if in.Kind == engineagents.InterruptProviderSwitched {
			consider(in.Detail, in.At)
		}
	}
	for _, p := range placements {
		consider(p.ProviderID, p.LastPlacedAt)
	}
	return providerID, found, nil
}
