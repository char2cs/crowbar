package conversation

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// MintChat creates chatID ("" mints one). providerID is written at birth, not left to
// the best-effort spawn, so even a chat whose spawn failed names its provider ("" for a
// placeholder row); surface is the view it is born on ("" for the provider's default).
func (c *Conversations) MintChat(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	surface string,
) (string, error) {
	if chatID == "" {
		chatID = uuid.NewString()
	}
	_, err := c.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Type:        domain.ChatTypeChat,
		ProviderID:  providerID,
		Surface:     surface,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	c.SeedPermissionLevel(ctx, chatID)
	return chatID, nil
}

// SeedPermissionLevel writes the current global default onto a new chat as a RAW,
// NON-explicit display value: spawns keep re-resolving the live default until the user
// picks a level. Best-effort — a failure must not stop the chat being created.
func (c *Conversations) SeedPermissionLevel(
	ctx context.Context,
	chatID string,
) {
	level, err := c.defaultPermissionLevel(ctx)
	if err != nil {
		return
	}
	if _, err := c.chats.SeedPermissionLevel(ctx, chatID, level); err != nil {
		slog.WarnContext(ctx, "agent: seed permission level (best-effort, continuing)",
			"chat_id", chatID, "err", err)
	}
}
