package conversation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
