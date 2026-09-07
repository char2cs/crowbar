package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// promptDelivery is what SubmitPrompt needs to restart a CLI carrying a
// message: the InjectSteps to render, and the resume/handoff signal those
// steps are chosen from.
type promptDelivery struct {
	promptSteps     []engineagents.InjectStep
	resumeSteps     []engineagents.InjectStep
	launchSessionID string
	resuming        bool
	// conversation and contextResuming carry a virgin-restart handoff: a native
	// session that has never itself recorded a turn holds no history for
	// --resume to restore, so restarting it to deliver its first real message
	// is this provider's first turn in the chat, not a gap since one it never
	// had. Both stay empty/false on every other path, where the mechanical
	// resuming above is already the right content signal too — see
	// resolvePromptDelivery.
	conversation    string
	contextResuming bool
}

func (rs *Runners) resolvePromptDelivery(
	ctx context.Context,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) (promptDelivery, error) {
	resuming, nativeSessionID, err := rs.resumeTarget(ctx, chatID, live)
	if err != nil {
		return promptDelivery{}, err
	}

	promptSteps, err := descriptor.PromptSteps(resuming)
	if err != nil {
		if errors.Is(err, engineagents.ErrPromptSubmitUnsupported) {
			return promptDelivery{}, ErrPromptUnsupported
		}
		return promptDelivery{}, fmt.Errorf("agent: submit prompt: render prompt mapping: %w", err)
	}
	if err := rs.RequirePromptRestart(ctx, chatID, live, descriptor); err != nil {
		return promptDelivery{}, err
	}

	// A named native session that has never itself recorded a turn has nothing
	// on disk for --resume to restore, and no native session at all is, a
	// fortiori, exactly as new — either way this restart is this provider's
	// FIRST real turn in the chat, not a gap since one it never had. Computed
	// before the resuming/not-resuming split below, and BEFORE any early
	// return, so both outcomes of resumeTarget get the same treatment: the
	// live bug this fixes reached here with resuming=false (resumeTarget's own
	// "not yet resumable" branch), not just the resuming=true branch. Same
	// test resumableConversation already uses for the switch path (see
	// switch.go), applied here for the restart-to-deliver path.
	everTurned := false
	if nativeSessionID != "" {
		_, found, err := rs.activity.LastTurnForSession(ctx, chatID, live.ProviderID, nativeSessionID)
		if err != nil {
			return promptDelivery{}, fmt.Errorf("agent: submit prompt: check native session history: %w", err)
		}
		everTurned = found
	}

	out := promptDelivery{promptSteps: promptSteps, resuming: resuming, contextResuming: resuming}
	if !everTurned {
		conversation, err := rs.conversations.AssembleConversation(ctx, chatID, false, time.Time{})
		if err != nil {
			return promptDelivery{}, fmt.Errorf("agent: submit prompt: assemble handoff: %w", err)
		}
		out.conversation = conversation
		out.contextResuming = false
	}
	if !resuming {
		return out, nil
	}
	// Unsupported is judged against the FULL native mapping, never the
	// api-transport-suppressed one below: a provider with no resume arg at
	// all cannot deliver this message any way, but one whose api connection
	// resumes it instead (nativeResumeSteps) is not that case merely because
	// its PTY carries no positional args.
	if _, resumable := descriptor.ResumeArg(); !resumable {
		return promptDelivery{}, ErrPromptUnsupported
	}
	out.resumeSteps = nativeResumeSteps(descriptor, nativeSessionID)
	out.launchSessionID = nativeSessionID
	return out, nil
}

func (rs *Runners) resumeTarget(
	ctx context.Context,
	chatID string,
	live engineagents.Runner,
) (bool, string, error) {
	if live.LaunchSessionID != "" &&
		(live.CurrentSession == "" || live.CurrentSession == live.LaunchSessionID) {
		return true, live.LaunchSessionID, nil
	}
	if live.CurrentSession == "" {
		return false, live.CurrentSession, nil
	}

	if live.CurrentSessionResumable || live.CurrentSessionSince.IsZero() {
		return live.CurrentSessionResumable, live.CurrentSession, nil
	}
	resuming, err := rs.activity.HasTurnAtOrAfter(ctx, chatID, live.ProviderID, live.CurrentSessionSince)
	if err != nil {
		return false, "", fmt.Errorf("agent: submit prompt: inspect current conversation: %w", err)
	}
	return resuming, live.CurrentSession, nil
}

func (rs *Runners) RequirePromptRestart(
	ctx context.Context,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) error {
	if descriptor.Capabilities().Delivery == engineagents.DeliveryRestartTUI {
		return nil
	}
	desired, err := rs.conversations.ChatSelection(ctx, chatID, false)
	if err != nil {
		return err
	}
	launched := engineagents.Selection{
		Model: live.LaunchModel, Effort: live.LaunchEffort, PermissionLevel: live.LaunchPermissionLevel,
	}
	if descriptor.SelectionRestart(launched, desired) {
		return nil
	}
	return ErrPromptUnsupported
}
