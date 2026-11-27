package runner

import (
	"cmp"
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) SubmitPromptWithSwitch(
	ctx context.Context,
	chatID, text, clientRequestID, provider string,
	selection *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: %w", err)
	}
	defer h.release()

	if err := rs.validateStagedSelection(ctx, chatID, provider, selection); err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	admit := func(chat domain.Chat, requestID string) (domain.AgentPromptSubmission, bool, error) {
		providerID, launches, err := rs.deliveryLaunch(ctx, chat, provider)
		if err != nil {
			return domain.AgentPromptSubmission{}, true, err
		}
		if !launches {
			return domain.AgentPromptSubmission{}, false, rs.commitStagedSelection(ctx, chatID, selection)
		}
		submission, err := rs.launchForDelivery(ctx, h, chat, providerID, requestID, text, selection)
		return submission, true, err
	}
	return rs.submitPromptLocked(ctx, h, chatID, text, clientRequestID, admit)
}

func (rs *Runners) commitStagedSelection(
	ctx context.Context, chatID string, selection *domain.ChatSelection,
) error {
	if selection == nil {
		return nil
	}
	if rs.holdsSelection(ctx, chatID, *selection) {
		return nil
	}
	if err := rs.requireChatAcceptingPrompt(ctx, chatID); err != nil {
		return err
	}
	return rs.setChatSelectionLocked(ctx, chatID, selection.Model, selection.Effort)
}

func (rs *Runners) SetChatSelection(ctx context.Context, chatID, model, effort string) error {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: set chat selection: %w", err)
	}
	defer h.release()
	if err := rs.validateStagedSelection(ctx, chatID, "", &domain.ChatSelection{Model: model, Effort: effort}); err != nil {
		return err
	}
	return rs.setChatSelectionLocked(ctx, chatID, model, effort)
}

func (rs *Runners) validateSelectionForProvider(
	ctx context.Context,
	chatID, providerID, model, effort string,
) error {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: validate selection: chat: %w", err)
	}
	agent, _, err := rs.promptAgent(ctx, chat, providerID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: validate selection: %w", err)
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

func (rs *Runners) holdsSelection(ctx context.Context, chatID string, selection domain.ChatSelection) bool {
	current, err := rs.conversations.ChatSelection(ctx, chatID)
	return err == nil && current.Model == selection.Model && current.Effort == selection.Effort
}

func (rs *Runners) setChatSelectionLocked(
	ctx context.Context,
	chatID, model, effort string,
) error {
	before, beforeErr := rs.conversations.ChatSelection(ctx, chatID)
	if err := rs.conversations.SetChatSelection(ctx, chatID, model, effort); err != nil {
		return err
	}
	st, err := rs.ledger.State(ctx, chatID)
	if _, turned := st.LastTurn(); beforeErr != nil || err != nil || !turned {
		return nil
	}
	if model != before.Model {
		rs.divide(ctx, chatID, ledger.MarkerModelChanged, model)
	}
	if effort != before.Effort && effort != cmp.Or(before.Effort, st.ReportedEffort()) {
		rs.divide(ctx, chatID, ledger.MarkerEffortChanged, effort)
	}
	return nil
}

func (rs *Runners) validateStagedSelection(
	ctx context.Context,
	chatID, provider string,
	selection *domain.ChatSelection,
) error {
	if selection == nil || *selection == (domain.ChatSelection{}) {
		return nil
	}
	var err error
	if provider == "" {
		provider, err = rs.conversations.ChatProviderID(ctx, chatID)
	}
	if err != nil {
		return err
	}
	return rs.validateSelectionForProvider(ctx, chatID, provider, selection.Model, selection.Effort)
}
