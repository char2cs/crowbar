package conversation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BackfillChatTypes records Type on every chat row minted before the field
// existed. Such a row replays with "" and the pre-audit daemon read it as a
// conversation (its EffectiveType); nothing reads "" that way any more, so it
// is written down once. A row whose history cannot be replayed is logged and
// counted in skipped; the error is for a list that failed or a cancelled ctx.
func (c *Conversations) BackfillChatTypes(ctx context.Context) (skipped int, err error) {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		return 0, fmt.Errorf("agent: backfill chat types: list: %w", err)
	}
	for _, row := range rows {
		if !untyped(row) {
			continue
		}
		_, err := c.chats.SetType(ctx, row.ID, domain.ChatTypeChat)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return skipped, fmt.Errorf("agent: backfill chat types: %w", ctxErr)
		}
		if err != nil {
			slog.WarnContext(ctx, "agent: backfill chat type: skipped a row", "chat_id", row.ID, "err", err)
			skipped++
		}
	}
	return skipped, nil
}

// UntypedChats lists what BackfillChatTypes leaves behind: the chats whose
// history replays to one still without a Type. A row that cannot be replayed
// is not listed; the backfill could not have typed it either.
func (c *Conversations) UntypedChats(ctx context.Context) ([]string, error) {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: untyped chats: list: %w", err)
	}
	var out []string
	for _, row := range rows {
		if !untyped(row) {
			continue
		}
		chat, err := c.chats.LoadChat(ctx, row.ID)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("agent: untyped chats: %w", ctxErr)
		}
		if err == nil && untyped(chat) {
			out = append(out, row.ID)
		}
	}
	return out, nil
}

func untyped(
	chat domain.Chat,
) bool {
	return chat.Type == ""
}
