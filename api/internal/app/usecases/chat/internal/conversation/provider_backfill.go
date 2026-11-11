package conversation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// RestateProvidersFromHistory makes domain.Chat.ProviderID agree, once per upgraded
// install, with newest (the provider the legacy history last names per chat); after it
// the field is the sole owner. Unrestatable rows are counted in skipped.
func (c *Conversations) RestateProvidersFromHistory(
	ctx context.Context,
	newest map[string]string,
) (skipped int, err error) {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent: restate chat providers: list: %w", err)
	}
	for _, row := range rows {
		err := c.restateProvider(ctx, row, newest)
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
// chats whose history replays to a provider newest disagrees with. A row that
// cannot be replayed is not listed; nothing could restate it.
func (c *Conversations) StaleProviders(ctx context.Context, newest map[string]string) ([]string, error) {
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
		if _, stale := staleProvider(chat, newest); stale {
			out = append(out, row.ID)
		}
	}
	return out, nil
}

func (c *Conversations) restateProvider(
	ctx context.Context,
	row domain.Chat,
	newest map[string]string,
) error {
	providerID, stale := staleProvider(row, newest)
	if !stale {
		return nil
	}
	if _, err := c.chats.SetProvider(ctx, row.ID, providerID); err != nil &&
		!errors.Is(err, asynxModels.ErrValidation) {
		return fmt.Errorf("set provider %q: %w", providerID, err)
	}
	return nil
}

// staleProvider is the provider newest names for chat when that is not the
// one chat records: what the restate acts on and StaleProviders checks.
func staleProvider(chat domain.Chat, newest map[string]string) (string, bool) {
	providerID := newest[chat.ID]
	if chat.Type == domain.ChatTypeFolder || providerID == "" || providerID == chat.ProviderID {
		return "", false
	}
	return providerID, true
}
