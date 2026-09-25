package runner

import (
	"context"
	"fmt"
	"log/slog"

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
// provider/selection: same contract as Usecase.SubmitPrompt's own doc —
// empty provider is "nothing staged, use current"; a non-empty one equal to
// the chat's current provider is a no-op (an ordinary resend must not pay for
// a switch it never asked for).
//
// A NON-NIL selection is committed whatever it holds. Gating on "either half
// is non-empty" instead — as this once did — made a pick of the provider's
// own default ("" on both halves, a real value, not silence) indistinguishable
// from staging nothing: the composer could stage "back to Default" and the
// chat would keep running the model it was already on, forever, with no
// gesture able to clear it except the separate PATCH the picker no longer
// makes. Only nil means "nothing staged".
func (rs *Runners) SubmitPromptWithSwitch(
	ctx context.Context,
	chatID, text, clientRequestID, provider string,
	selection *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	park, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	defer release()

	switching := false
	if provider != "" {
		current, err := rs.conversations.ChatProviderID(ctx, chatID)
		if err != nil {
			return domain.AgentPromptSubmission{}, err
		}
		switching = provider != current
	}

	if err := rs.validateStagedSelection(ctx, chatID, provider, selection); err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	if switching {
		defer rs.enterPhase(ctx, chatID, rs.replacementPhase(ctx, chatID))()
		if _, err := rs.switchProviderLocked(ctx, park, chatID, provider); err != nil {
			return domain.AgentPromptSubmission{}, err
		}
	}

	if selection != nil {
		if err := rs.setChatSelectionLocked(ctx, chatID, selection.Model, selection.Effort); err != nil {
			return domain.AgentPromptSubmission{}, err
		}
	}
	// Send is the only intent: a dormant chat is revived here, never by a client.
	revive := func() error {
		if err := rs.reviveForDelivery(ctx, park, chatID); err != nil {
			return parkErr(park, err)
		}
		return nil
	}
	return rs.submitPromptLocked(ctx, chatID, text, clientRequestID, revive)
}

// SetChatSelection is the standalone PATCH .../selection route's entry
// point — gated by the SAME rs.spawns hold every other mutating path on this
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
	_, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return err
	}
	defer release()
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
