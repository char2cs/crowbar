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

// BackfillProviders writes domain.Chat.ProviderID for every conversation row
// that does not carry one yet — rows minted before the field existed — so the
// field can be the ONE owner of "as whom does this chat come back" (spec §7-A
// target 2). Everything after this reads the field; nothing re-derives it.
//
// Run at boot. It costs one list read when every row already carries its
// vendor, which after the first boot is every row that has ever had a CLI.
// Best-effort per row: a row it cannot resolve stays unset and reads as "no
// provider recorded", exactly as a chat that never ran.
func (c *Conversations) BackfillProviders(ctx context.Context) error {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("agent: backfill chat providers: list: %w", err)
	}
	for _, row := range rows {
		if row.ProviderID != "" || row.EffectiveType() == domain.ChatTypeFolder {
			continue
		}
		providerID, found, err := c.newestPlacedProvider(ctx, row.ID)
		if err != nil {
			slog.WarnContext(ctx, "agent: backfill chat provider (best-effort, continuing)",
				"chat_id", row.ID, "err", err)
			continue
		}
		if !found {
			continue
		}
		if _, err := c.chats.SetProvider(ctx, row.ID, providerID); err != nil &&
			!errors.Is(err, asynxModels.ErrValidation) {
			slog.WarnContext(ctx, "agent: backfill chat provider: set (best-effort, continuing)",
				"chat_id", row.ID, "provider", providerID, "err", err)
		}
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
