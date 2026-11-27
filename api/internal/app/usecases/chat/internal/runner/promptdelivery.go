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

type sessionLaunch struct {
	resumeID  string
	assign    *ledger.Row
	afterLoss bool
	relayID   string
}

func launchSession(st ledger.State, d engineagents.Agent) (sessionLaunch, error) {
	c, ok := st.Conversation(d.ID())
	if ok && c.Resumable() {
		return sessionLaunch{resumeID: c.SessionID, relayID: c.RelayID}, nil
	}
	launch := sessionLaunch{afterLoss: ok && c.State == ledger.ConversationLost}
	if d.SessionPinsRelay() {
		launch.relayID = uuid.NewString()
	}
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

func (rs *Runners) planLaunch(ctx context.Context, chatID string, d engineagents.Agent) (sessionLaunch, error) {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return sessionLaunch{}, fmt.Errorf("agent: plan launch: %w", err)
	}
	return launchSession(st, d)
}

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

type promptDelivery struct {
	provider        string
	descriptor      engineagents.Agent
	outgoing        engineagents.Runner
	selection       *domain.ChatSelection
	promptSteps     []engineagents.InjectStep
	launch          sessionLaunch
	conversation    string
	contextResuming bool
}

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

	apiOnly := descriptor.APIOnlyPrompts()
	var promptSteps []engineagents.InjectStep
	if !apiOnly {
		promptSteps, err = descriptor.PromptSteps(resuming)
	}
	if err != nil {
		if errors.Is(err, engineagents.ErrPromptSubmitUnsupported) {
			return promptDelivery{}, ErrPromptUnsupported
		}
		return promptDelivery{}, fmt.Errorf("agent: submit prompt: render prompt mapping: %w", err)
	}
	if !apiOnly {
		if err := rs.RequirePromptRestart(ctx, chatID, live, descriptor); err != nil {
			return promptDelivery{}, err
		}
	}

	everTurned := resuming
	leftAt, err := rs.leftAt(ctx, chatID, providerID, nativeSessionID)
	if err != nil {
		return promptDelivery{}, err
	}

	turned, err := rs.turnedSinceOutgoing(ctx, chatID, providerID, live)
	if err != nil {
		return promptDelivery{}, err
	}

	out := promptDelivery{
		provider: providerID, descriptor: descriptor, outgoing: live, promptSteps: promptSteps,
		launch: launch, contextResuming: resuming,
	}
	if !turned {
		conversation, err := rs.conversations.AssembleConversation(ctx, chatID, everTurned, leftAt)
		if err != nil {
			return promptDelivery{}, fmt.Errorf("agent: submit prompt: assemble handoff: %w", err)
		}
		out.conversation = conversation
		out.contextResuming = everTurned
	}
	if !resuming || apiOnly {
		return out, nil
	}
	if _, resumable := descriptor.ResumeArg(); !resumable {
		return promptDelivery{}, ErrPromptUnsupported
	}
	return out, nil
}

func (rs *Runners) turnedSinceOutgoing(
	ctx context.Context, chatID, providerID string, outgoing engineagents.Runner,
) (bool, error) {
	if outgoing.ID == "" || outgoing.ProviderID != providerID {
		return false, nil
	}
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: submit prompt: check live runner history: %w", err)
	}
	_, turned := st.RunnerTurn(outgoing.ID)
	return turned, nil
}

func (rs *Runners) launchCarrying(
	ctx context.Context, h *hold, chat domain.Chat, requestID, text string, delivery promptDelivery,
) (domain.AgentPromptSubmission, error) {
	carrier := uuid.NewString()
	rs.live.launching.Store(carrier, true)
	defer rs.live.launching.Delete(carrier)
	note, err := rs.recordPrompt(ctx, chat.ID, requestID, text, "", ledger.DeliveryRestart, carrier, nil)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	delivery, err = rs.displaceForDelivery(ctx, h, chat.ID, requestID, delivery)
	if err != nil {
		return domain.AgentPromptSubmission{}, rs.failPrompt(ctx, chat.ID, requestID, err)
	}
	runnerID, err := rs.spawnRunner(ctx, chat.ID, chat.WorkspaceID, delivery.provider, spawnInput{
		runnerID: carrier, launch: &delivery.launch, finalSteps: delivery.promptSteps,
		conversation: delivery.conversation, resuming: delivery.contextResuming, promptMessage: text, moveNote: note,
		selection: delivery.selection,
	})
	if err != nil {
		return domain.AgentPromptSubmission{}, rs.failPrompt(ctx, chat.ID, requestID, spawnFailure(ctx, err))
	}
	return rs.commitPromptSpawn(ctx, chat.ID, requestID, runnerID)
}

func (rs *Runners) displaceForDelivery(
	ctx context.Context, h *hold, chatID, requestID string, d promptDelivery,
) (promptDelivery, error) {
	if d.outgoing.ID == "" {
		return d, nil
	}
	if err := rs.displaceForPrompt(ctx, h, chatID, requestID, d.outgoing.ID); err != nil {
		return d, err
	}
	replanned, err := rs.resolvePromptDelivery(ctx, chatID, d.provider, d.outgoing, d.descriptor)
	replanned.selection = d.selection
	return replanned, err
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

func (rs *Runners) selectionRequiresRestart(
	ctx context.Context,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) (bool, error) {
	desired, err := rs.conversations.ChatSelection(ctx, chatID)
	if err != nil {
		return false, err
	}
	launched := engineagents.Selection{
		Model: live.LaunchModel, Effort: live.LaunchEffort, PermissionLevel: live.LaunchPermissionLevel,
	}
	return descriptor.SelectionRestart(launched, desired), nil
}
