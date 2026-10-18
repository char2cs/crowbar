package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

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
	// unspoken is the bound session the chat never turned on: its CLI never
	// wrote it, so a provider that takes an id is given it again, not resumed.
	unspoken string
}

// launchSession is the launch decision: the provider's bound session is
// resumed by its id, and only by its id; an unbound or lost one starts anew.
func launchSession(st ledger.State, d engineagents.Agent) (sessionLaunch, error) {
	c, ok := st.Conversation(d.ID())
	if ok && c.State == ledger.ConversationBound {
		return sessionLaunch{resumeID: c.SessionID}, nil
	}
	launch := sessionLaunch{lost: ok && c.State == ledger.ConversationLost}
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

// planLaunch reads chatID's ledger for launchSession. A provider that takes an
// id is resumed only on a session the chat turned on: its CLI writes the session
// at the first turn. Any other provider is resumed and refuses what it lacks.
func (rs *Runners) planLaunch(ctx context.Context, chatID string, d engineagents.Agent) (sessionLaunch, error) {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return sessionLaunch{}, fmt.Errorf("agent: plan launch: %w", err)
	}
	launch, err := launchSession(st, d)
	if _, assigns := d.SessionAssignArg(); err != nil || launch.resumeID == "" || !assigns {
		return launch, err
	}
	_, turned, err := rs.activity.LastTurnForSession(ctx, chatID, d.ID(), launch.resumeID)
	if err != nil {
		return sessionLaunch{}, fmt.Errorf("agent: plan launch: turns on the session: %w", err)
	}
	if turned {
		return launch, nil
	}
	c, _ := st.Conversation(d.ID())
	return sessionLaunch{unspoken: launch.resumeID, lost: c.Cause == ledger.CauseFresh}, nil
}

// steps are the argv steps that put the CLI on its session: the resume arg,
// or the assign arg naming the minted or unspoken id. A provider that takes no
// id launches fresh and announces its own.
func (l sessionLaunch) steps(d engineagents.Agent) ([]engineagents.InjectStep, error) {
	if l.resumeID != "" {
		return resumeInjectionSteps(d, l.resumeID), nil
	}
	id := l.unspoken
	if l.assign != nil {
		bound, err := ledger.DecodeBody[ledger.SessionBound](*l.assign)
		if err != nil {
			return nil, fmt.Errorf("agent: assign session: %w", err)
		}
		id = bound.SessionID
	}
	if id == "" {
		return nil, nil
	}
	arg, _ := d.SessionAssignArg()
	return sessionArgSteps(arg, id), nil
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

	// everTurned/leftAt answer "does the NATIVE SESSION have history of its
	// own" — true whenever ANY runner ever recorded a turn under it, which
	// says nothing about whether the runner THIS restart is replacing was the
	// one that produced it. A provider switch spawns its own silent resume
	// runner to compute and carry a handoff, then this restart immediately
	// displaces it to actually deliver the prompt — so nativeSessionID here
	// routinely already has turns from BEFORE that runner ever ran a single
	// one. Gating on everTurned alone (as this once did) reads that prior
	// history as "already caught up" and skips the handoff entirely — live-
	// confirmed as the reason a switch-then-send never reached Claude: the
	// switch's own assembled context was thrown away with the runner it was
	// injected into, and this restart's replacement carried none.
	everTurned := false
	var leftAt time.Time
	if nativeSessionID != "" {
		at, found, err := rs.activity.LastTurnForSession(ctx, chatID, live.ProviderID, nativeSessionID)
		if err != nil {
			return promptDelivery{}, fmt.Errorf("agent: submit prompt: check native session history: %w", err)
		}
		everTurned, leftAt = found, at
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
