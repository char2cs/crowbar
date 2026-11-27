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
	c.seedPermissionLevel(ctx, chatID)
	return chatID, nil
}

func (c *Conversations) seedPermissionLevel(
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
