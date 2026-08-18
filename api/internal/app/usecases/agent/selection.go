package agent

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// SetChatSelection writes a chat's sticky choice of model and reasoning effort.
//
// Both values are written together, and BOTH may be empty: empty is the
// provider's own default, so clearing is a legitimate selection rather than a
// missing one. Nothing is substituted — a cleared chat spawns the argv it would
// have spawned before any of this existed.
//
// A non-empty value is checked against the DECLARED catalogue of the provider
// currently on the chat, and an unknown one is refused outright
// (apperr.ErrInvalidArgument → 400). The alternative — accepting it and letting
// the CLI reject it at spawn — turns a typo into a chat whose next message kills
// its own process, which is a far worse place to learn about it.
//
// The effort catalogue is read for the model being SET, not the one currently
// stored, because both fields move in one call: an effort level that is only
// valid for the incoming model must validate against that model.
func (u *Usecase) SetChatSelection(
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

func (u *Usecase) validateSelection(
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

// chatAgent resolves the agent whose catalogue a selection is judged against:
// the provider of the runner live on the chat, falling back to the provider of
// its last conversation.
//
// That is the same "which provider is this chat's" answer the chat DTO reports
// (dto.activeProviderID), and it must be, or a picker filled from one catalogue
// would be validated against another.
//
// A chat that has never had a runner has no provider and therefore no catalogue
// to check against. It is refused rather than waved through: there is no picker
// on such a chat, so a selection arriving for one is a client bug, and accepting
// it would store a value nothing can ever validate.
func (u *Usecase) chatAgent(
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

func (u *Usecase) chatProviderID(
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

// chatSelection reads the choice a spawn or a prompt must honour.
//
// It folds the EVENT LOG (LoadChat) rather than reading the projection, for the
// same reason a spawn resolving a thread's parent does: the projection is
// asynchronous, and a user who picks a model and immediately sends a message
// would otherwise have the message delivered under the model they just changed
// away from.
//
// minting is the chat-does-not-exist-yet case — SpawnChat writes the aggregate
// AFTER the CLI is live — and a chat that does not exist has chosen nothing.
func (u *Usecase) chatSelection(
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

// resolveEfforts flattens an agent's effort catalogue into one entry per
// selectable model, plus "" for the provider's own default.
//
// The descriptor's model-independent fallback is applied HERE rather than left on
// the wire, so a client reads efforts[model] and is done: the one place that
// knows the fallback rule is the engine, and it stays there.
//
// A model with no levels is OMITTED rather than mapped to an empty list. codex is
// exactly that case for the "" key: its catalogue is per-model with no fallback,
// so its default model has no declared levels and a null entry would only invite
// a client to render an empty picker for it.
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
