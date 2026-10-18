package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// sessionLaunch is how one launch continues the chat's conversation with its
// provider: resuming the bound session, or starting one — minted here when the
// provider takes an id at launch, announced by the CLI otherwise.
type sessionLaunch struct {
	resumeID string
	// assign binds the session Crowbar minted; it is appended before the fork.
	assign *ledger.Row
	// lost: the provider refused the chat's session, so the new one starts
	// with no memory of the conversation and is handed none until it turns.
	lost bool
}

// launchSession is the launch decision. A bound session that turned is resumed
// by its id, and only by its id. Any other starts anew: one that never turned
// was never written by its CLI, so a new id replaces it rather than reusing it.
func launchSession(st ledger.State, d engineagents.Agent) (sessionLaunch, error) {
	c, ok := st.Conversation(d.ID())
	if ok && c.Resumable() {
		return sessionLaunch{resumeID: c.SessionID}, nil
	}
	launch := sessionLaunch{lost: ok && (c.State == ledger.ConversationLost || c.Cause == ledger.CauseFresh)}
	if _, ok := d.SessionAssignArg(); !ok {
		return launch, nil
	}
	row, err := ledger.Assign(st, d.ID(), uuid.NewString())
	if err != nil {
		return sessionLaunch{}, fmt.Errorf("agent: assign session: %w", err)
	}
	launch.assign = &row
	return launch, nil
}

// planLaunch reads chatID's ledger for launchSession.
func (rs *Runners) planLaunch(ctx context.Context, chatID string, d engineagents.Agent) (sessionLaunch, error) {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return sessionLaunch{}, fmt.Errorf("agent: plan launch: %w", err)
	}
	return launchSession(st, d)
}

// steps are the argv steps that put the CLI on its session: the resume arg,
// or the assign arg naming the minted id. A provider that takes no id launches
// fresh and announces its own.
func (l sessionLaunch) steps(d engineagents.Agent) ([]engineagents.InjectStep, error) {
	if l.resumeID != "" {
		return resumeInjectionSteps(d, l.resumeID), nil
	}
	if l.assign == nil {
		return nil, nil
	}
	bound, err := ledger.DecodeBody[ledger.SessionBound](*l.assign)
	if err != nil {
		return nil, fmt.Errorf("agent: assign session: %w", err)
	}
	arg, _ := d.SessionAssignArg()
	return sessionArgSteps(arg, bound.SessionID), nil
}

// promptDelivery is what SubmitPrompt needs to restart a CLI carrying a
// message: the InjectSteps to render, and the session those steps continue.
type promptDelivery struct {
	promptSteps []engineagents.InjectStep
	launch      sessionLaunch
	// conversation and contextResuming carry a handoff for a restart whose
	// LIVE runner (the one being displaced to deliver this message) has never
	// itself turned — its first delivery, whether that runner is brand new to
	// the chat or was just spawned by a provider switch and displaced before
	// it got to answer. Both stay empty/false on every other path, where the
	// mechanical resuming above is already the right content signal too — see
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
	launch, err := rs.planLaunch(ctx, chatID, descriptor)
	if err != nil {
		return promptDelivery{}, err
	}
	resuming, nativeSessionID := launch.resumeID != "", launch.resumeID

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

	// Only a session that turned is resumed, so a resumed one has history of its
	// own; the gap since its last turn is what it lacks. Whether THIS restart owes
	// that handoff is liveTurned's question below.
	everTurned := resuming
	leftAt, err := rs.leftAt(ctx, chatID, live.ProviderID, nativeSessionID)
	if err != nil {
		return promptDelivery{}, err
	}

	// liveTurned is the question that actually decides whether THIS restart
	// owes a handoff: has the runner being displaced right now itself ever
	// turned, since it was spawned. A runner that has is mid-conversation on
	// its own native transcript, which --resume already restores in full. A
	// runner that has not — a switch's silent resume, or a dormant chat's
	// revive spawn, neither ever given a chance to answer before being
	// displaced here — has nothing of its own on the wire, so this restart is
	// really that spawn's first delivery and must carry what one would have:
	// the gap since leftAt when the session has prior history (everTurned),
	// or the conversation so far when it has none at all.
	liveTurned, err := rs.activity.HasTurnAtOrAfter(ctx, chatID, live.ProviderID, live.StartedAt)
	if err != nil {
		return promptDelivery{}, fmt.Errorf("agent: submit prompt: check live runner history: %w", err)
	}

	out := promptDelivery{promptSteps: promptSteps, launch: launch, contextResuming: resuming}
	if !liveTurned && !launch.lost {
		conversation, err := rs.conversations.AssembleConversation(ctx, chatID, everTurned, leftAt)
		if err != nil {
			return promptDelivery{}, fmt.Errorf("agent: submit prompt: assemble handoff: %w", err)
		}
		out.conversation = conversation
		out.contextResuming = everTurned
	}
	if !resuming {
		return out, nil
	}
	// Unsupported is judged against the FULL native mapping: a provider with no
	// resume arg at all cannot deliver this message any way, but one whose api
	// connection may resume it instead is not that case merely because its PTY
	// might end up carrying no positional args.
	if _, resumable := descriptor.ResumeArg(); !resumable {
		return promptDelivery{}, ErrPromptUnsupported
	}
	return out, nil
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
	changed, err := rs.selectionRequiresRestart(ctx, chatID, live, descriptor)
	if err != nil {
		return err
	}
	if changed {
		return nil
	}
	return ErrPromptUnsupported
}

// selectionRequiresRestart reports whether live's LAUNCHED model/effort/
// permission level differ from the chat's CURRENT desired selection in a
// field the descriptor itself declares restart_tui for (descriptor.
// SelectionRestart) — the same comparison RequirePromptRestart already made
// for the restart-per-prompt path, extracted so submitPromptOverAPI's live
// connection shortcut (prompts.go) can refuse it and fall back to an actual
// restart instead of silently delivering to a process whose model/effort no
// longer matches what the user just asked for.
func (rs *Runners) selectionRequiresRestart(
	ctx context.Context,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) (bool, error) {
	desired, err := rs.conversations.ChatSelection(ctx, chatID, false)
	if err != nil {
		return false, err
	}
	launched := engineagents.Selection{
		Model: live.LaunchModel, Effort: live.LaunchEffort, PermissionLevel: live.LaunchPermissionLevel,
	}
	return descriptor.SelectionRestart(launched, desired), nil
}
