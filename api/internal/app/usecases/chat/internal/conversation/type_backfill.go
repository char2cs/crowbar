package conversation

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// BackfillChatTypes records Type on every chat row minted before the field
// existed. Such a row replays with "" and the pre-audit daemon read it as a
// conversation (its EffectiveType); nothing reads "" that way any more, so it
// is written down once. Best-effort per row; reports whether every row landed.
func (c *Conversations) BackfillChatTypes(ctx context.Context) bool {
	rows, err := c.chats.ListChats(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "agent: backfill chat types: list", "err", err)
		return false
	}
	complete := true
	for _, row := range rows {
		if row.Type != "" {
			continue
		}
		if _, err := c.chats.SetType(ctx, row.ID, domain.ChatTypeChat); err != nil {
			slog.WarnContext(ctx, "agent: backfill chat type (best-effort, continuing)",
				"chat_id", row.ID, "err", err)
			complete = false
		}
	}
	return complete
}
