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
// the one owner and nothing re-derives it. A row that cannot be restated is
// logged and counted in skipped; the error is for a failed list or a cancelled
// ctx.
func (c *Conversations) RestateProvidersFromHistory(ctx context.Context) (skipped int, err error) {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent: restate chat providers: list: %w", err)
	}
	for _, row := range rows {
		err := c.restateProvider(ctx, row)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return skipped, fmt.Errorf("agent: restate chat providers: %w", ctxErr)
		}
		if err != nil {
			slog.WarnContext(ctx, "agent: restate chat provider: skipped a row", "chat_id", row.ID, "err", err)
			skipped++
		}
	}
	return skipped, nil
}

// StaleProviders lists what RestateProvidersFromHistory leaves behind: the
// chats whose history replays to a provider their runner history disagrees
// with. A row that cannot be replayed is not listed; nothing could restate it.
func (c *Conversations) StaleProviders(ctx context.Context) ([]string, error) {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: stale chat providers: list: %w", err)
	}
	var out []string
	for _, row := range rows {
		chat, err := c.chats.LoadChat(ctx, row.ID)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("agent: stale chat providers: %w", ctxErr)
		}
		if err != nil {
			continue
		}
		_, stale, err := c.staleProvider(ctx, chat)
		if err != nil {
			return nil, fmt.Errorf("agent: stale chat providers: chat %s: %w", row.ID, err)
		}
		if stale {
			out = append(out, row.ID)
		}
	}
	return out, nil
}

func (c *Conversations) restateProvider(
	ctx context.Context,
	row domain.Chat,
) error {
	providerID, stale, err := c.staleProvider(ctx, row)
	if err != nil || !stale {
		return err
	}
	if _, err := c.chats.SetProvider(ctx, row.ID, providerID); err != nil &&
		!errors.Is(err, asynxModels.ErrValidation) {
		return fmt.Errorf("set provider %q: %w", providerID, err)
	}
	return nil
}

// staleProvider is the provider chat's runner history names when that is not
// the one chat records: what the restate acts on and StaleProviders checks.
func (c *Conversations) staleProvider(
	ctx context.Context,
	chat domain.Chat,
) (string, bool, error) {
	if chat.Type == domain.ChatTypeFolder {
		return "", false, nil
	}
	providerID, found, err := c.newestPlacedProvider(ctx, chat.ID)
	if err != nil || !found || providerID == "" || providerID == chat.ProviderID {
		return "", false, err
	}
	return providerID, true, nil
}

// newestPlacedProvider is the one-time migration of the old projection scan:
// whichever of the chat's provider-switch interruptions and runner placements
// is NEWEST names the provider last running here.
func (c *Conversations) newestPlacedProvider(
	ctx context.Context,
	chatID string,
) (providerID string, found bool, err error) {
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
