package runner

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/domain"
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

// promptDelivery is what SubmitPrompt needs to launch a CLI carrying a
// message: the InjectSteps to render, and the session those steps continue.
type promptDelivery struct {
	provider    string // the provider the carrier launches as
	outgoing    string // the chat's CLI the carrier replaces, "" on a dormant chat
	switching   bool   // the carrier changes the chat's provider
	promptSteps []engineagents.InjectStep
	launch      sessionLaunch
	// conversation and contextResuming hand the record over to a launch whose
	// outgoing runner never turned, or that has none (resolvePromptDelivery).
	conversation    string
	contextResuming bool
}

// resolvePromptDelivery plans the launch of providerID's CLI carrying a
// prompt; live is the chat's CLI it replaces, zero on a dormant chat.
func (rs *Runners) resolvePromptDelivery(
	ctx context.Context,
	chatID, providerID string,
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
	leftAt, err := rs.leftAt(ctx, chatID, providerID, nativeSessionID)
	if err != nil {
		return promptDelivery{}, err
	}

	// The launch owes the handoff unless the runner it displaces turned since it
	// started: that one is on its own transcript, which --resume restores, while
	// one that never answered, or none at all, has nothing of its own on the wire.
	liveTurned, err := rs.liveTurned(ctx, chatID, providerID, live)
	if err != nil {
		return promptDelivery{}, err
	}

	out := promptDelivery{
		provider: providerID, outgoing: live.ID, promptSteps: promptSteps, launch: launch, contextResuming: resuming,
	}
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

// liveTurned reports whether live, the CLI a delivery replaces, has turned
// since it started; a dormant chat has none.
func (rs *Runners) liveTurned(ctx context.Context, chatID, providerID string, live engineagents.Runner) (bool, error) {
	if live.ID == "" {
		return false, nil
	}
	turned, err := rs.activity.HasTurnAtOrAfter(ctx, chatID, providerID, live.StartedAt)
	if err != nil {
		return false, fmt.Errorf("agent: submit prompt: check live runner history: %w", err)
	}
	return turned, nil
}

// launchCarrying launches the CLI that carries requestID, recorded as its
// carrier before anything is touched, once the CLI it replaces has quit. A
// switch is recorded once that CLI runs.
func (rs *Runners) launchCarrying(
	ctx context.Context, h *hold, chat domain.Chat, requestID, text string, delivery promptDelivery,
) (domain.AgentPromptSubmission, error) {
	carrier := uuid.NewString()
	if err := rs.recordPrompt(ctx, chat.ID, requestID, text, "", ledger.DeliveryRestart, carrier, nil); err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	if delivery.outgoing != "" {
		if err := rs.displaceForPrompt(ctx, h, chat.ID, requestID, delivery.outgoing); err != nil {
			return domain.AgentPromptSubmission{}, rs.failPrompt(ctx, chat.ID, requestID, err)
		}
	}
	runnerID, err := rs.spawnRunner(ctx, chat.ID, chat.WorkspaceID, delivery.provider, spawnInput{
		runnerID: carrier, launch: &delivery.launch, finalSteps: delivery.promptSteps,
		conversation: delivery.conversation, resuming: delivery.contextResuming, promptMessage: text,
	}, false)
	if err != nil {
		return domain.AgentPromptSubmission{}, rs.failPrompt(ctx, chat.ID, requestID, spawnFailure(ctx, err))
	}
	if delivery.switching {
		rs.recordProviderSwitch(ctx, chat.ID, delivery.provider)
	}
	return rs.commitPromptSpawn(ctx, chat.ID, requestID, runnerID)
}

func (rs *Runners) promptTarget(
	ctx context.Context,
	chat domain.Chat,
) (engineagents.Runner, engineagents.Agent, string, error) {
	live, ok := rs.live.byChat(chat.ID)
	if !ok {
		return engineagents.Runner{}, nil, "", ErrPromptSessionUnavailable
	}
	descriptor, worktree, err := rs.promptAgent(ctx, chat, live.ProviderID)
	return live, descriptor, worktree, err
}

// promptAgent is providerID's descriptor and the worktree chat's CLI runs in.
func (rs *Runners) promptAgent(
	ctx context.Context, chat domain.Chat, providerID string,
) (engineagents.Agent, string, error) {
	cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chat.ID, chat.WorkspaceID)
	if err != nil {
		return nil, "", err
	}
	crowbarHome, _, _, worktree, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
	if err != nil {
		return nil, "", fmt.Errorf("agent: submit prompt: worktree dir: %w", err)
	}
	descriptor, err := rs.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		return nil, "", fmt.Errorf("agent: submit prompt: resolve descriptor: %w", err)
	}
	return descriptor, worktree, nil
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
