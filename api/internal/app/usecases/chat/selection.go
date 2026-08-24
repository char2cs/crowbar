package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (u *chatUsecase) SetChatSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) error {
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return fmt.Errorf("agent: set chat selection: chat: %w", err)
	}
	if model != "" || effort != "" {
		if err := u.validateSelection(ctx, chatID, model, effort); err != nil {
			return err
		}
	}
	if _, err := u.chats.SetSelection(ctx, chatID, model, effort); err != nil {
		return fmt.Errorf("agent: set chat selection: save: %w", err)
	}
	return nil
}

func (u *chatUsecase) validateSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) error {
	agent, err := u.chatAgent(ctx, chatID)
	if err != nil {
		return err
	}
	if model != "" && !contains(agent.Models(), model) {
		return fmt.Errorf("agent: set chat selection: %q declares no model %q: %w",
			agent.ID(), model, apperr.ErrInvalidArgument)
	}
	if effort != "" && !contains(agent.Efforts(model), effort) {
		return fmt.Errorf("agent: set chat selection: %q declares no effort %q for model %q: %w",
			agent.ID(), effort, model, apperr.ErrInvalidArgument)
	}
	return nil
}

func (u *chatUsecase) chatAgent(
	ctx context.Context,
	chatID string,
) (engineagents.Agent, error) {
	providerID, err := u.chatProviderID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	home, err := u.home()
	if err != nil {
		return nil, fmt.Errorf("agent: chat agent: home: %w", err)
	}
	agent, err := u.agents.Get(ctx, home, providerID)
	if err != nil {
		return nil, fmt.Errorf("agent: chat agent: resolve descriptor: %w", err)
	}
	return agent, nil
}

func (u *chatUsecase) chatProviderID(
	ctx context.Context,
	chatID string,
) (string, error) {
	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ProviderID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: chat provider: live runner: %w", err)
	}
	last, err := u.runners.LastConversation(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: chat provider: no provider has ever run on this chat: %w",
			apperr.ErrUnprocessable)
	}
	if err != nil {
		return "", fmt.Errorf("agent: chat provider: last conversation: %w", err)
	}
	return last.ProviderID, nil
}

func (u *chatUsecase) chatSelection(
	ctx context.Context,
	chatID string,
	minting bool,
) (engineagents.Selection, error) {
	if minting {
		return engineagents.Selection{}, nil
	}
	chat, err := u.chats.LoadChat(ctx, chatID)
	if err != nil {
		return engineagents.Selection{}, fmt.Errorf("agent: chat selection: %w", err)
	}
	return engineagents.Selection{Model: chat.Model, Effort: chat.Effort}, nil
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func resolveEfforts(
	agent engineagents.Agent,
) map[string][]string {
	if !agent.Capabilities().EffortSelect {
		return nil
	}
	out := map[string][]string{}
	for _, model := range append([]string{""}, agent.Models()...) {
		if levels := agent.Efforts(model); len(levels) > 0 {
			out[model] = levels
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
