package runner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// SubmitPromptWithSwitch sends with the provider and pick a composer staged, in
// one hold of the chat: the pick is checked before anything changes, and every
// decision after a wait is taken again. Only a nil selection stages nothing.
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

	// Admission runs once the request is known to be new: a replayed or busy
	// send must leave the chat's provider and selection exactly as it found them.
	admit := func(chat domain.Chat, requestID string) (domain.AgentPromptSubmission, bool, error) {
		providerID, launches, err := rs.deliveryLaunch(ctx, chat, provider)
		if err != nil {
			return domain.AgentPromptSubmission{}, true, err
		}
		if !launches {
			// The live CLI's own delivery is decided against the chat's pick.
			return domain.AgentPromptSubmission{}, false, rs.commitStagedSelection(ctx, chatID, selection)
		}
		submission, err := rs.launchForDelivery(ctx, h, chat, providerID, requestID, text, selection)
		return submission, true, err
	}
	return rs.submitPromptLocked(ctx, h, chatID, text, clientRequestID, admit)
}

// commitStagedSelection commits a staged pick only for a chat that will take
// the send; nil means nothing was staged.
func (rs *Runners) commitStagedSelection(
	ctx context.Context, chatID string, selection *domain.ChatSelection,
) error {
	if selection == nil {
		return nil
	}
	// The composer re-posts its picker on every send. A pick the chat already
	// holds changes nothing, so it must not turn a deliverable send away as busy.
	if rs.holdsSelection(ctx, chatID, *selection) {
		return nil
	}
	if err := rs.requireChatAcceptingPrompt(ctx, chatID); err != nil {
		return err
	}
	return rs.setChatSelectionLocked(ctx, chatID, selection.Model, selection.Effort)
}

// SetChatSelection is the standalone PATCH .../selection route's entry
// point — under the SAME hold of the chat every other mutating path on this
// interface takes (SwitchProvider, StopChat, SwitchToTerminal, SwitchToNative,
// SubmitPromptWithSwitch), so it can never interleave its own read-before /
// write / record-diff trio with one of THEIRS. Before this existed, the
// Usecase called conversation.Conversations.SetChatSelection directly, with
// no lock at all: a standalone selection change racing a SubmitPrompt call
// that staged its own model/effort could each read a "before" snapshot the
// other's write had not landed yet, so the durable interruption log ended up
// narrating a transition to a value a concurrent write immediately
// superseded. See
// TestRegression_SetChatSelection_ConcurrentStandaloneAndStagedNeverLogAStaleChange
// (chat_test.go) for the property this closes off.
func (rs *Runners) SetChatSelection(ctx context.Context, chatID, model, effort string) error {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: set chat selection: %w", err)
	}
	defer h.release()
	return rs.setChatSelectionLocked(ctx, chatID, model, effort)
}

// resolveTargetProvider is the provider a staged model/effort must validate
// against: the one this call is itself about to switch to, or — nothing
// staged — the chat's current one.
func (rs *Runners) resolveTargetProvider(ctx context.Context, chatID, staged string) (string, error) {
	if staged != "" {
		return staged, nil
	}
	return rs.conversations.ChatProviderID(ctx, chatID)
}

// validateSelectionForProvider mirrors conversation.Conversations'
// unexported validateSelection, against an EXPLICIT provider id rather than
// the chat's current one — conversation.Conversations can't be asked "would
// this be valid for provider X" without first making X the chat's current
// provider, which is exactly the commit this call must happen before.
func (rs *Runners) validateSelectionForProvider(
	ctx context.Context,
	chatID, providerID, model, effort string,
) error {
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: validate selection: chat: %w", err)
	}
	crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: validate selection: worktree dir: %w", err)
	}
	agent, err := rs.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: validate selection: resolve descriptor: %w", err)
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

// holdsSelection reports whether the chat's pick already is selection.
func (rs *Runners) holdsSelection(ctx context.Context, chatID string, selection domain.ChatSelection) bool {
	current, err := rs.conversations.ChatSelection(ctx, chatID, false)
	return err == nil && current.Model == selection.Model && current.Effort == selection.Effort
}

// setChatSelectionLocked commits model/effort and records a divider for each
// half that changed. It repeats Usecase.recordSelectionChange (turn.go), which
// hangs off the Usecase, where Runners cannot reach it.
func (rs *Runners) setChatSelectionLocked(
	ctx context.Context,
	chatID, model, effort string,
) error {
	before, beforeErr := rs.conversations.ChatSelection(ctx, chatID, false)
	if err := rs.conversations.SetChatSelection(ctx, chatID, model, effort); err != nil {
		return err
	}
	if beforeErr != nil {
		return nil
	}
	if model != before.Model {
		if err := rs.turns.RecordChatSwitch(
			ctx, chatID, engineagents.InterruptModelChanged, model,
		); err != nil {
			slog.WarnContext(ctx, "agent: submit prompt: record model change (best-effort, continuing)",
				"chat_id", chatID, "err", err)
		}
	}
	if effort != before.Effort {
		if err := rs.turns.RecordChatSwitch(
			ctx, chatID, engineagents.InterruptEffortChanged, effort,
		); err != nil {
			slog.WarnContext(ctx, "agent: submit prompt: record effort change (best-effort, continuing)",
				"chat_id", chatID, "err", err)
		}
	}
	return nil
}

// validateStagedSelection refuses a staged model/effort BEFORE the switch
// commits — the same "refuse before anything is torn down" rule
// switchProviderLocked's own disabled-provider guard follows. Checked against
// the provider this call is ABOUT to switch to, not the one SetChatSelection
// would resolve AFTER the switch already ran: validating there is what let a
// bad pick strand the chat on a freshly switched-to provider with nothing
// delivered and no way back but another switch.
//
// A nil selection is "nothing staged" and validates trivially.
func (rs *Runners) validateStagedSelection(
	ctx context.Context,
	chatID, provider string,
	selection *domain.ChatSelection,
) error {
	if selection == nil {
		return nil
	}
	targetProvider, err := rs.resolveTargetProvider(ctx, chatID, provider)
	if err != nil {
		return err
	}
	return rs.validateSelectionForProvider(
		ctx, chatID, targetProvider, selection.Model, selection.Effort,
	)
}
