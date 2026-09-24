package conversation

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
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
			agent.ID(), model, apperr.ErrInvalidArgument)
	}
	if effort != "" && !engineagents.Allowed(agent.Efforts(model), discovered, effort) {
		return fmt.Errorf("agent: set chat selection: %q declares no effort %q for model %q: %w",
			agent.ID(), effort, model, apperr.ErrInvalidArgument)
	}
	return nil
}

// SetChatPermissionLevel pins the chat's own guarded/trusted/full-auto
// override, refusing a level the chat's CURRENT provider does not declare —
// never clamping to a neighboring one, per the same "if a provider can't
// reach a level, it is not offered" rule the switcher's own options list
// enforces. A provider that later replaces this one may accept a level this
// one refused; nothing here freezes the choice against a future switch.
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
			agent.ID(), level, apperr.ErrInvalidArgument)
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

// ChatProviderID resolves the provider a chat's next CLI should be — the same
// engineagents.ResolveProviderID answer dto.activeProviderID and Resume derive, so
// the selection/spawn path (chatAgent, SetChatSelection, SwitchProvider's
// previousProviderID, SubmitPrompt's implicit-provider prompt) can never spawn a
// dormant chat as the wrong vendor. Refuses with apperr.ErrUnprocessable when no
// provider has EVER run on the chat — there is nothing to resolve to.
//
// The chat's own durable vendor is the last source, and it is why
// SwitchProvider's previousProviderID is now knowable for a chat that bound
// nothing: an unresolvable previous provider is what made a real conversion
// record no provider_switched marker at all, so it left no trace anywhere.
func (c *Conversations) ChatProviderID(
	ctx context.Context,
	chatID string,
) (string, error) {
	live, err := c.runnerStore.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ProviderID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: chat provider: live runner: %w", err)
	}

	var conversations []engineagents.ChatConversation
	last, err := c.runnerStore.LastConversation(ctx, chatID)
	switch {
	case err == nil:
		conversations = []engineagents.ChatConversation{last}
	case errors.Is(err, agentrunner.ErrNotFound):
		// No conversation ever bound — a switch interruption may still answer it.
	default:
		return "", fmt.Errorf("agent: chat provider: last conversation: %w", err)
	}

	interruptions, err := c.activity.Interruptions(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: chat provider: interruptions: %w", err)
	}

	chat, err := c.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: chat provider: chat: %w", err)
	}

	providerID, found := engineagents.ResolveProviderID(conversations, interruptions, chat.ProviderID)
	if !found {
		return "", fmt.Errorf("agent: chat provider: no provider has ever run on this chat: %w",
			apperr.ErrUnprocessable)
	}
	return providerID, nil
}

// ChatSelection reads what a chat WANTS to run its provider as. minting=true
// is for a chat that has no row yet (mid-mint, its runner's argv already
// being rendered): Model/Effort correctly read as "the provider's own
// default" (their own always-valid empty state), but PermissionLevel has no
// such reading — a spawn with none at all would run whatever the provider's
// own untouched default turns out to be, exactly the ungoverned behavior
// this feature exists to replace. So minting resolves the CURRENT global
// default directly, falling back to guarded (never full-auto) on a lookup
// failure — the one place this package still knows that name, because
// nothing durable exists yet to read it from.
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
		// A human chose this level FOR THIS CHAT (SetChatPermissionLevel): it
		// wins over the global dial for good, whatever the dial says now.
	case level != "":
		// INHERITED, never pinned — the common case. The value seeded at mint
		// (or by the legacy branch below) is a display snapshot, not a spawn
		// intent: re-resolve the CURRENT global default on every spawn, so a
		// later change to the dial reaches every chat that never opted out of
		// it. Confirmed live: a chat left on "inherit" kept answering from
		// whatever the default was the day it was minted, forever — this is
		// the actual reported bug ("I set my global permissions to full-auto
		// and my chats don't use it"). A lookup failure degrades to the
		// last-seeded value rather than erroring the spawn.
		if cur, cerr := c.defaultPermissionLevel(ctx); cerr == nil {
			level = cur
		}
	default:
		// Should never happen — domain.Chat's own doc comment says a chat is
		// always seeded with a real level at creation — except for one that
		// predates the seeding logic and was carried through unseeded (a chat
		// from before this feature existed, replayed through the chat-model
		// migration that never had this field at all). Read as "not seeded
		// yet", never as a genuine choice: resolve the CURRENT global default
		// here, the same as a fresh mint, and seed it (still non-explicit) so
		// this chat stops being unseeded — though the case above would keep
		// tracking the live default for it either way.
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
