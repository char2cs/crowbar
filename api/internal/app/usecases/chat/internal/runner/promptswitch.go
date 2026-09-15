package runner

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// SubmitPromptWithSwitch is the combined "switch, stage a selection, then
// send" a real composer click performs in one gesture — Usecase.SubmitPrompt's
// own entry point, now delegated here so the whole sequence runs under ONE
// rs.spawns.Lock(chatID) hold, the same Lock-once-call-the-locked-form shape
// SwitchProvider itself already uses.
//
// Before this existed, Usecase.SubmitPrompt ran the switch (a full
// SwitchProvider call — its own Lock, fully released on return) and the
// delivery (Runners.SubmitPrompt — a SECOND, separate Lock) as two
// back-to-back but UNLOCKED-IN-BETWEEN steps. Nothing held the chat between
// them, so a second caller — another SubmitPrompt, a bare SwitchProvider —
// could land its own switch in that gap and steal the delivery: the message
// the user sent to their just-picked provider was delivered to whichever
// provider happened to be live when the SECOND Lock was finally acquired,
// not the one this call had just switched to. See
// TestRegression_SubmitPromptWithStagedProvider_ConcurrentSendsNeverCrossDeliver
// (runner_test.go) for the black-box property this closes off.
//
// provider/model/effort: same contract as Usecase.SubmitPrompt's own doc —
// empty provider is "nothing staged, use current"; a non-empty one equal to
// the chat's current provider is a no-op (an ordinary resend must not pay for
// a switch it never asked for).
func (rs *Runners) SubmitPromptWithSwitch(
	ctx context.Context,
	chatID, text, clientRequestID, provider, model, effort string,
) (domain.AgentPromptSubmission, error) {
	defer rs.spawns.Lock(chatID)()

	switching := false
	if provider != "" {
		current, err := rs.conversations.ChatProviderID(ctx, chatID)
		if err != nil {
			return domain.AgentPromptSubmission{}, err
		}
		switching = provider != current
	}

	if model != "" || effort != "" {
		targetProvider, err := rs.resolveTargetProvider(ctx, chatID, provider)
		if err != nil {
			return domain.AgentPromptSubmission{}, err
		}
		// VALIDATED BEFORE THE SWITCH COMMITS — the same "refuse before
		// anything is torn down" rule switchProviderLocked's own disabled-
		// provider guard follows. Checked against the provider this call is
		// ABOUT to switch to, not the one SetChatSelection would resolve
		// AFTER the switch already ran: validating there instead is what let
		// a bad model/effort pick strand the chat on a freshly switched-to
		// provider with nothing delivered and no way back but another
		// switch — the switch had already committed by the time the bad
		// value was even noticed.
		if err := rs.validateSelectionForProvider(ctx, chatID, targetProvider, model, effort); err != nil {
			return domain.AgentPromptSubmission{}, err
		}
	}

	if switching {
		if _, err := rs.switchProviderLocked(ctx, chatID, provider); err != nil {
			return domain.AgentPromptSubmission{}, err
		}
	}

	if model != "" || effort != "" {
		if err := rs.setChatSelectionLocked(ctx, chatID, model, effort); err != nil {
			return domain.AgentPromptSubmission{}, err
		}
	}

	return rs.submitPromptLocked(ctx, chatID, text, clientRequestID)
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
	if model != "" && !slices.Contains(agent.Models(), model) {
		return fmt.Errorf("agent: set chat selection: %q declares no model %q: %w",
			agent.ID(), model, apperr.ErrInvalidArgument)
	}
	if effort != "" && !slices.Contains(agent.Efforts(model), effort) {
		return fmt.Errorf("agent: set chat selection: %q declares no effort %q for model %q: %w",
			agent.ID(), effort, model, apperr.ErrInvalidArgument)
	}
	return nil
}

// setChatSelectionLocked commits model/effort and records whichever of the
// two actually changed — mirrors Usecase.recordSelectionChange (turn.go),
// duplicated rather than shared because that method hangs off the Usecase,
// not anything Runners can reach: same reasoning as MergedSeparator
// (spawnsteps.go)'s own duplicated literal.
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
