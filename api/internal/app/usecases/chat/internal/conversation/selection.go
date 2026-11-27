package conversation

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (c *Conversations) SetChatSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) error {
	if _, err := c.chats.GetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent: set chat selection: chat: %w", err)
	}
	if _, err := c.chats.SetSelection(ctx, chatID, model, effort); err != nil {
		return fmt.Errorf("agent: set chat selection: save: %w", err)
	}
	return nil
}

func (c *Conversations) SetChatPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) error {
	agent, err := c.chatAgent(ctx, chatID)
	if err != nil {
		return err
	}
	if !contains(agent.PermissionLevels(), level) {
		return fmt.Errorf("agent: set chat permission level: %q declares no level %q: %w",
			agent.ID(), level, problem.ErrInvalidArgument)
	}
	if _, err := c.chats.SetPermissionLevel(ctx, chatID, level); err != nil {
		return fmt.Errorf("agent: set chat permission level: save: %w", err)
	}
	return nil
}

func (c *Conversations) chatAgent(
	ctx context.Context,
	chatID string,
) (engineagents.Agent, error) {
	providerID, err := c.ChatProviderID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	home, err := c.home()
	if err != nil {
		return nil, fmt.Errorf("agent: chat agent: home: %w", err)
	}
	agent, err := c.agents.Get(ctx, home, providerID)
	if err != nil {
		return nil, fmt.Errorf("agent: chat agent: resolve descriptor: %w", err)
	}
	return agent, nil
}

func (c *Conversations) ChatProviderID(
	ctx context.Context,
	chatID string,
) (string, error) {
	if live, err := c.runners.LiveRunnerForChat(ctx, chatID); err == nil {
		return live.ProviderID, nil
	}
	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: chat provider: chat: %w", err)
	}
	if chat.ProviderID == "" {
		return "", fmt.Errorf("agent: chat provider: no provider has ever run on this chat: %w",
			problem.ErrUnprocessable)
	}
	return chat.ProviderID, nil
}

func (c *Conversations) ChatSelection(
	ctx context.Context,
	chatID string,
) (engineagents.Selection, error) {
	chat, err := c.chats.LoadChat(ctx, chatID)
	if err != nil {
		return engineagents.Selection{}, fmt.Errorf("agent: chat selection: %w", err)
	}
	level := chat.PermissionLevel
	switch {
	case chat.PermissionLevelExplicit:
	case level != "":
		if cur, cerr := c.defaultPermissionLevel(ctx); cerr == nil {
			level = cur
		}
	default:
		level, err = c.defaultPermissionLevel(ctx)
		if err != nil {
			level = "guarded"
		} else {
			c.seedPermissionLevel(ctx, chatID)
		}
	}
	return engineagents.Selection{
		Model: chat.Model, Effort: chat.Effort, PermissionLevel: level,
	}, nil
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
