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
	if model != "" || effort != "" {
		if err := c.validateSelection(ctx, chatID, model, effort); err != nil {
			return err
		}
	}
	if _, err := c.chats.SetSelection(ctx, chatID, model, effort); err != nil {
		return fmt.Errorf("agent: set chat selection: save: %w", err)
	}
	return nil
}

func (c *Conversations) validateSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) error {
	agent, err := c.chatAgent(ctx, chatID)
	if err != nil {
		return err
	}
	discovered := agent.Capabilities().ModelDiscovery
	if model != "" && !engineagents.Allowed(agent.Models(), discovered, model) {
		return fmt.Errorf("agent: set chat selection: %q declares no model %q: %w",
			agent.ID(), model, problem.ErrInvalidArgument)
	}
	if effort != "" && !engineagents.Allowed(agent.Efforts(model), discovered, effort) {
		return fmt.Errorf("agent: set chat selection: %q declares no effort %q for model %q: %w",
			agent.ID(), effort, model, problem.ErrInvalidArgument)
	}
	return nil
}

// SetChatPermissionLevel pins the chat's own guarded/trusted/full-auto override,
// refusing (never clamping) a level the chat's CURRENT provider does not declare,
// the same rule the switcher's options follow.
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

// ChatProviderID resolves the provider a chat's next CLI should be: the live
// runner's (mid-switch, the incoming one), else domain.Chat.ProviderID.
// problem.ErrUnprocessable when no provider has ever been recorded.
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

// ChatSelection reads what a chat WANTS to run its provider as. minting=true (no row
// yet) still needs a permission level, since none would leave the provider ungoverned:
// it resolves the global default, falling back to guarded, never full-auto.
func (c *Conversations) ChatSelection(
	ctx context.Context,
	chatID string,
	minting bool,
) (engineagents.Selection, error) {
	if minting {
		level, err := c.defaultPermissionLevel(ctx)
		if err != nil {
			level = "guarded"
		}
		return engineagents.Selection{PermissionLevel: level}, nil
	}
	chat, err := c.chats.LoadChat(ctx, chatID)
	if err != nil {
		return engineagents.Selection{}, fmt.Errorf("agent: chat selection: %w", err)
	}
	level := chat.PermissionLevel
	switch {
	case chat.PermissionLevelExplicit:
		// A human chose this level FOR THIS CHAT: it wins over the global dial for good.
	case level != "":
		// INHERITED: the seeded value is a display snapshot, so re-resolve the CURRENT
		// global default on every spawn and a dial change reaches every chat that never
		// opted out. A lookup failure degrades to the seeded value.
		if cur, cerr := c.defaultPermissionLevel(ctx); cerr == nil {
			level = cur
		}
	default:
		// A legacy chat that predates seeding: "not seeded yet", never a choice.
		// Resolve the global default as a fresh mint does, and seed it (non-explicit).
		level, err = c.defaultPermissionLevel(ctx)
		if err != nil {
			level = "guarded"
		} else {
			c.SeedPermissionLevel(ctx, chatID)
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
