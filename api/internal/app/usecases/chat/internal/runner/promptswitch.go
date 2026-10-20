package runner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// SubmitPromptWithSwitch is the combined "stage a provider and a selection,
// then send" a real composer click performs in one gesture —
// Usecase.SubmitPrompt's own entry point, now delegated here so the whole
// sequence runs under ONE hold of the chat, the same hold-once-call-the-
// locked-form shape SwitchProvider itself already uses. Its waits release the
// chat's lock, and every decision after one is taken again.
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
		switching, err := rs.stagesASwitch(ctx, chatID, provider)
		if err != nil {
			return domain.AgentPromptSubmission{}, true, err
		}
		if switching {
			submission, err := rs.switchForDelivery(ctx, h, chat, provider, requestID, text, selection)
			return submission, true, err
		}
		if err := rs.commitStagedSelection(ctx, chatID, provider, selection); err != nil {
			return domain.AgentPromptSubmission{}, true, err
		}
		// Send is the only intent: a dormant chat is revived here, never by a client.
		return rs.reviveForDelivery(ctx, h, chat, requestID, text)
	}
	return rs.submitPromptLocked(ctx, h, chatID, text, clientRequestID, admit)
}

// stagesASwitch reports whether provider, staged with a send, is not the chat's own.
func (rs *Runners) stagesASwitch(ctx context.Context, chatID, provider string) (bool, error) {
	if provider == "" {
		return false, nil
	}
	current, err := rs.conversations.ChatProviderID(ctx, chatID)
	return err == nil && provider != current, err
}

// switchForDelivery launches providerID carrying requestID, in place of the
// chat's CLI if it has one: the switch is that one launch, never a silent
// spawn the delivery then displaces.
func (rs *Runners) switchForDelivery(
	ctx context.Context, h *hold, chat domain.Chat, providerID, requestID, text string,
	selection *domain.ChatSelection,
) (domain.AgentPromptSubmission, error) {
	defer rs.enterPhase(ctx, chat.ID, rs.replacementPhase(chat.ID))()
	delivery, err := rs.planCarrying(ctx, chat, providerID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	if err := rs.commitStagedSelection(ctx, chat.ID, providerID, selection); err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	delivery.switching = true
	return rs.launchCarrying(ctx, h, chat, requestID, text, delivery)
}

// commitStagedSelection commits a staged pick, judged against the staged
// provider, only for a chat that will take the send; nil means nothing was staged.
func (rs *Runners) commitStagedSelection(
	ctx context.Context, chatID, staged string, selection *domain.ChatSelection,
) error {
	if selection == nil {
		return nil
	}
	// The composer re-posts its picker on every send. A pick the chat already
	// holds changes nothing, so it must not turn a deliverable send away as busy.
	if current, err := rs.conversations.ChatSelection(ctx, chatID, false); err == nil &&
		current.Model == selection.Model && current.Effort == selection.Effort {
		return nil
	}
	if err := rs.requireChatAcceptingPrompt(ctx, chatID); err != nil {
		return err
	}
	return rs.setChatSelectionLocked(ctx, chatID, staged, selection.Model, selection.Effort)
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
	return rs.setChatSelectionLocked(ctx, chatID, "", model, effort)
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

// setChatSelectionLocked commits model/effort, judged against staged or else
// the chat's own provider, and records whichever of the two actually
// changed — mirrors Usecase.recordSelectionChange (turn.go), duplicated rather
// than shared because that method hangs off the Usecase, not anything Runners
// can reach: same reasoning as MergedSeparator (spawnsteps.go)'s own
// duplicated literal.
func (rs *Runners) setChatSelectionLocked(
	ctx context.Context,
	chatID, staged, model, effort string,
) error {
	providerID, err := rs.selectionProvider(ctx, chatID, staged, model, effort)
	if err != nil {
		return err
	}
	before, beforeErr := rs.conversations.ChatSelection(ctx, chatID, false)
	if err := rs.conversations.SetChatSelection(ctx, chatID, providerID, model, effort); err != nil {
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

// selectionProvider is the provider a pick is judged against: the one staged
// with it, else the chat's own. Clearing a pick is judged by none.
func (rs *Runners) selectionProvider(ctx context.Context, chatID, staged, model, effort string) (string, error) {
	if model == "" && effort == "" {
		return staged, nil
	}
	return rs.resolveTargetProvider(ctx, chatID, staged)
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
