package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const (
	defaultMessagePageLimit = 100
	maxMessagePageLimit     = 200
	maxReactPromptBytes     = 64 * 1024
)

func (u *runnerUsecase) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID string,
) (domain.AgentPromptSubmission, error) {
	clientRequestID, err := normalisePromptRequest(text, clientRequestID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	textHash := agentjournal.PromptTextHash(text)

	defer u.spawns.lock(chatID)()

	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: chat: %w", err)
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: chats dir: %w", err)
	}
	journalDir := u.prompts.Dir(chatsDir, chat.ID)
	if err := u.reconcilePendingPromptFromLedger(ctx, chat); err != nil {
		slog.ErrorContext(ctx, "agent: reconcile React prompt before submission (best-effort)",
			"chat_id", chat.ID, "err", err)
	}

	result, done, err := u.replayPriorAttempt(ctx, chat, journalDir, clientRequestID, textHash)
	if done {
		return result, err
	}

	live, descriptor, err := u.promptTarget(ctx, chat)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	delivery, err := u.resolvePromptDelivery(ctx, chat.ID, live, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	if err := u.requirePromptIdle(ctx, chatID, live.ID); err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	replacementRunnerID := uuid.NewString()
	prior, existingAttempt, err := u.prompts.Begin(
		journalDir, clientRequestID, textHash, live.ProviderID, live.ID, replacementRunnerID, time.Now(),
	)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf(
			"agent: submit prompt: begin durable dispatch: %w", promptJournalError(err),
		)
	}
	if existingAttempt {
		result, done, classifyErr := u.classifyPriorAttempt(
			ctx, chat, journalDir, clientRequestID, prior,
		)
		if done {
			return result, classifyErr
		}
		return domain.AgentPromptSubmission{}, ErrPromptOutcomeUnknown
	}
	if err := u.displaceForPrompt(ctx, chatID, journalDir, clientRequestID, live.ID); err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	runnerID, err := u.spawnRunner(
		ctx, chatID, chat.WorkspaceID, live.ProviderID, replacementRunnerID,
		delivery.resumeSteps, delivery.promptSteps, "", 0,
		delivery.resuming, delivery.launchSessionID, false, text,
	)
	if err != nil {
		if errors.Is(err, engineterminal.ErrCommandNotFound) {
			_ = u.prompts.MarkFailedDispatch(journalDir, clientRequestID, time.Now())
			return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: replacement spawn: %w", err)
		}
		return domain.AgentPromptSubmission{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "replacement spawn", err,
		)
	}
	return u.commitPromptSpawn(ctx, journalDir, clientRequestID, textHash, runnerID)
}

func normalisePromptRequest(
	text, clientRequestID string,
) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("agent: submit prompt: text is required: %w", apperr.ErrInvalidArgument)
	}
	if len([]byte(text)) > maxReactPromptBytes {
		return "", fmt.Errorf("agent: submit prompt: text exceeds %d bytes: %w",
			maxReactPromptBytes, apperr.ErrInvalidArgument)
	}

	if strings.IndexByte(text, 0) >= 0 {
		return "", fmt.Errorf("agent: submit prompt: text contains an unsupported NUL byte: %w",
			apperr.ErrInvalidArgument)
	}
	requestUUID, err := uuid.Parse(clientRequestID)
	if err != nil {
		return "", fmt.Errorf("agent: submit prompt: clientRequestId must be a UUID: %w",
			apperr.ErrInvalidArgument)
	}
	return requestUUID.String(), nil
}

func (u *runnerUsecase) replayPriorAttempt(
	ctx context.Context,
	chat domain.AgentChat,
	journalDir, clientRequestID, textHash string,
) (domain.AgentPromptSubmission, bool, error) {
	existing, found, err := u.prompts.Lookup(journalDir, clientRequestID, textHash)
	err = promptJournalError(err)
	if errors.Is(err, ErrPromptRequestIDConflict) {
		return domain.AgentPromptSubmission{}, true, err
	}
	if err != nil {
		return domain.AgentPromptSubmission{}, true, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "idempotency lookup", err,
		)
	}
	if !found {
		return domain.AgentPromptSubmission{}, false, nil
	}
	return u.classifyPriorAttempt(ctx, chat, journalDir, clientRequestID, existing)
}

func (u *runnerUsecase) promptTarget(
	ctx context.Context,
	chat domain.AgentChat,
) (engineagents.Runner, engineagents.Agent, error) {
	live, err := u.runners.LiveRunnerForChat(ctx, chat.ID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return engineagents.Runner{}, nil, ErrPromptSessionUnavailable
	}
	if err != nil {
		return engineagents.Runner{}, nil, fmt.Errorf("agent: submit prompt: live runner: %w", err)
	}
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return engineagents.Runner{}, nil, fmt.Errorf("agent: submit prompt: worktree dir: %w", err)
	}
	descriptor, err := u.agents.Get(ctx, crowbarHome, live.ProviderID)
	if err != nil {
		return engineagents.Runner{}, nil, fmt.Errorf("agent: submit prompt: resolve descriptor: %w", err)
	}
	return live, descriptor, nil
}

func (u *runnerUsecase) displaceForPrompt(
	ctx context.Context,
	chatID, journalDir, clientRequestID, liveRunnerID string,
) error {
	unlockTurnStart := u.turnStarts.lock(chatID)
	defer unlockTurnStart()

	if err := u.requirePromptIdle(ctx, chatID, liveRunnerID); err != nil {
		_ = u.prompts.MarkFailedDispatch(journalDir, clientRequestID, time.Now())
		return err
	}

	if err := u.quitOutgoingCLI(ctx, chatID); err != nil {
		_ = u.prompts.MarkFailedDispatch(journalDir, clientRequestID, time.Now())
		return err
	}
	return nil
}

func (u *runnerUsecase) commitPromptSpawn(
	ctx context.Context,
	journalDir, clientRequestID, textHash, runnerID string,
) (domain.AgentPromptSubmission, error) {
	spawned, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		return domain.AgentPromptSubmission{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "read replacement runner", err,
		)
	}
	result := domain.AgentPromptSubmission{
		RunnerID:          runnerID,
		TerminalSessionID: spawned.TerminalSession,
	}
	if result.TerminalSessionID == "" {
		return domain.AgentPromptSubmission{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "replacement terminal identity is missing", nil,
		)
	}
	committed, err := u.prompts.MarkSpawned(
		journalDir, clientRequestID, textHash, result.RunnerID, result.TerminalSessionID, time.Now(),
	)
	if err != nil {
		return domain.AgentPromptSubmission{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "persist replacement identity", err,
		)
	}
	if committed.State == agentjournal.PromptStateUncertain {
		return domain.AgentPromptSubmission{}, ErrPromptOutcomeUnknown
	}
	return result, nil
}

func uncertainPromptOutcome(ctx context.Context, stage string, cause error) error {
	attrs := []any{"stage", stage}
	if cause != nil {
		attrs = append(attrs, "err", cause)
	}
	slog.ErrorContext(ctx, "agent: React prompt delivery outcome is uncertain", attrs...)
	return ErrPromptOutcomeUnknown
}

func (u *runnerUsecase) markPromptOutcomeUncertain(
	ctx context.Context,
	journalDir, clientRequestID, stage string,
	cause error,
) error {
	if err := u.prompts.MarkUncertain(journalDir, clientRequestID, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: persist uncertain React prompt outcome",
			"stage", stage, "client_request_id", clientRequestID, "err", err)
	}
	return uncertainPromptOutcome(ctx, stage, cause)
}

func (u *runnerUsecase) requirePromptIdle(ctx context.Context, chatID, runnerID string) error {
	current, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return ErrPromptSessionUnavailable
	}
	if err != nil {
		return fmt.Errorf("agent: submit prompt: refresh live runner: %w", err)
	}
	if current.ID != runnerID {
		return ErrPromptBusy
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: refresh chat: %w", err)
	}

	if chat.Working || len(u.turns.inflight(chatID)) > 0 {
		return ErrPromptBusy
	}
	return nil
}

func (u *runnerUsecase) requireNoPendingPromptDelivery(ctx context.Context, chat domain.AgentChat) error {
	if err := u.reconcilePendingPromptFromLedger(ctx, chat); err != nil {
		return fmt.Errorf("agent: prompt delivery guard: reconcile ledger evidence: %w", err)
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: prompt delivery guard: chats dir: %w", err)
	}
	pending, err := u.prompts.HasPendingDelivery(u.prompts.Dir(chatsDir, chat.ID))
	if err != nil {
		return fmt.Errorf("agent: prompt delivery guard: inspect journal: %w", err)
	}
	if pending {
		return ErrPromptBusy
	}
	return nil
}

func (u *runnerUsecase) reconcilePromptJournalsOnBoot(ctx context.Context) error {
	chats, err := u.chats.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile prompt journals: list chats: %w", err)
	}
	dirs := make(map[string]string)
	now := time.Now()
	for _, chat := range chats {
		chatsDir, ok := dirs[chat.WorkspaceID]
		if !ok {
			chatsDir, err = u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
			if err != nil {
				return fmt.Errorf("agent: boot reconcile prompt journals: chats dir for %s: %w", chat.ID, err)
			}
			dirs[chat.WorkspaceID] = chatsDir
		}
		dir := u.prompts.Dir(chatsDir, chat.ID)
		if err := u.prompts.RecoverOrphanedDispatches(dir, now); err != nil {
			return fmt.Errorf("agent: boot reconcile prompt journal for %s: %w", chat.ID, err)
		}
	}
	return nil
}

func resumeInjectionSteps(d engineagents.Agent, sessionID string) []engineagents.InjectStep {
	arg, ok := d.ResumeArg()
	if sessionID == "" || !ok {
		return nil
	}
	ctx := engineagents.TemplateCtx{ID: sessionID}
	parts := strings.Fields(engineagents.Expand(arg, ctx))
	steps := make([]engineagents.InjectStep, 0, len(parts))
	for _, part := range parts {
		steps = append(steps, engineagents.InjectStep{
			Verb: "pass_arg",
			Args: map[string]any{"positional": part},
		})
	}
	return steps
}

func promptSubmission(record agentjournal.PromptRequest) domain.AgentPromptSubmission {
	return domain.AgentPromptSubmission{
		RunnerID:          record.RunnerID,
		TerminalSessionID: record.TerminalSessionID,
	}
}

func (u *runnerUsecase) classifyPriorAttempt(
	ctx context.Context,
	chat domain.AgentChat,
	journalDir, clientRequestID string,
	existing agentjournal.PromptRequest,
) (domain.AgentPromptSubmission, bool, error) {
	if existing.RunnerID != "" && existing.TerminalSessionID != "" &&
		(existing.State == agentjournal.PromptStateSpawned || existing.State == agentjournal.PromptStateAccepted) {
		return promptSubmission(existing), true, nil
	}
	if existing.State == agentjournal.PromptStateDispatching ||
		existing.State == agentjournal.PromptStateSpawned ||
		existing.State == agentjournal.PromptStateUncertain {
		return u.recoverPriorDelivery(ctx, chat, journalDir, clientRequestID, existing)
	}
	if existing.State == agentjournal.PromptStateAccepted {
		return domain.AgentPromptSubmission{}, true, ErrPromptAlreadyAccepted
	}
	return domain.AgentPromptSubmission{}, false, nil
}

func (u *runnerUsecase) recoverPriorDelivery(
	ctx context.Context,
	chat domain.AgentChat,
	journalDir, clientRequestID string,
	existing agentjournal.PromptRequest,
) (domain.AgentPromptSubmission, bool, error) {
	accepted, err := u.promptRecordAccepted(ctx, chat, existing)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "recover prior delivery", err,
		)
	}
	if !accepted {
		return domain.AgentPromptSubmission{}, true, ErrPromptOutcomeUnknown
	}
	if _, err := u.prompts.MarkAccepted(journalDir, clientRequestID, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: submit prompt: persist recovered acceptance",
			"chat_id", chat.ID, "client_request_id", clientRequestID, "err", err)
	}
	return domain.AgentPromptSubmission{}, true, ErrPromptAlreadyAccepted
}

func (u *runnerUsecase) promptRecordAccepted(
	ctx context.Context,
	chat domain.AgentChat,
	record agentjournal.PromptRequest,
) (bool, error) {
	turns, err := u.chat.chatTurns(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: recover prompt request: turns: %w", err)
	}
	for _, t := range turns {
		if deliveredThisRequest(t, record) && agentjournal.PromptTextHash(t.Text) == record.TextHash {
			return true, nil
		}
	}
	return false, nil
}

func deliveredThisRequest(
	turn chatlog.Turn,
	record agentjournal.PromptRequest,
) bool {
	if turn.Role != "user" || turn.Provider != record.ProviderID || turn.At.Before(record.CreatedAt) {
		return false
	}
	if record.RunnerID != "" {
		return turn.RunnerID == record.RunnerID
	}
	return turn.RunnerID != "" && turn.RunnerID != record.OutgoingRunnerID
}

func (u *runnerUsecase) reconcilePendingPromptFromLedger(
	ctx context.Context,
	chat domain.AgentChat,
) error {
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("reconcile prompt acceptance: chats dir: %w", err)
	}
	dir := u.prompts.Dir(chatsDir, chat.ID)
	record, found, err := u.prompts.ActiveDelivery(dir)
	if err != nil || !found {
		return err
	}
	accepted, err := u.promptRecordAccepted(ctx, chat, record)
	if err != nil || !accepted {
		return err
	}
	if _, err := u.prompts.MarkAccepted(dir, record.RequestID, time.Now()); err != nil {
		return fmt.Errorf("reconcile prompt acceptance: persist: %w", err)
	}
	return nil
}

func (u *runnerUsecase) confirmPromptAccepted(
	ctx context.Context,
	chat domain.AgentChat,
	runner engineagents.Runner,
	text string,
) error {
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("prompt journal chats dir: %w", err)
	}
	dir := u.prompts.Dir(chatsDir, chat.ID)
	return u.prompts.ConfirmAccepted(
		dir, runner.ID, runner.ProviderID, agentjournal.PromptTextHash(text), time.Now(),
	)
}

func (u *runnerUsecase) reconcilePromptRunnerDeparture(
	ctx context.Context,
	runner engineagents.Runner,
	chatID string,
) {
	if chatID == "" {
		return
	}
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return
	}
	dir := u.prompts.Dir(chatsDir, chat.ID)
	record, found, err := u.prompts.ActiveForRunner(dir, runner.ID, runner.ProviderID)
	if err != nil || !found {
		return
	}
	accepted, err := u.promptRecordAccepted(ctx, chat, record)
	if err != nil {
		return
	}
	if accepted {
		_, _ = u.prompts.MarkAccepted(dir, record.RequestID, time.Now())
		return
	}

	_ = u.prompts.MarkUncertain(dir, record.RequestID, time.Now())
}

type promptDelivery struct {
	promptSteps     []engineagents.InjectStep
	resumeSteps     []engineagents.InjectStep
	launchSessionID string
	resuming        bool
}

func (u *runnerUsecase) resolvePromptDelivery(
	ctx context.Context,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) (promptDelivery, error) {
	resuming, nativeSessionID, err := u.resumeTarget(ctx, chatID, live)
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
	if err := u.requirePromptRestart(ctx, chatID, live, descriptor); err != nil {
		return promptDelivery{}, err
	}
	out := promptDelivery{promptSteps: promptSteps, resuming: resuming}
	if !resuming {
		return out, nil
	}
	out.resumeSteps = resumeInjectionSteps(descriptor, nativeSessionID)
	if len(out.resumeSteps) == 0 {
		return promptDelivery{}, ErrPromptUnsupported
	}
	out.launchSessionID = nativeSessionID
	return out, nil
}

func (u *runnerUsecase) resumeTarget(
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
	resuming, err := u.activity.HasTurnAtOrAfter(ctx, chatID, live.ProviderID, live.CurrentSessionSince)
	if err != nil {
		return false, "", fmt.Errorf("agent: submit prompt: inspect current conversation: %w", err)
	}
	return resuming, live.CurrentSession, nil
}

func (u *runnerUsecase) requirePromptRestart(
	ctx context.Context,
	chatID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
) error {
	if descriptor.Capabilities().Delivery == engineagents.DeliveryRestartTUI {
		return nil
	}
	desired, err := u.chat.chatSelection(ctx, chatID, false)
	if err != nil {
		return err
	}
	launched := engineagents.Selection{Model: live.LaunchModel, Effort: live.LaunchEffort}
	if descriptor.SelectionRestart(launched, desired) {
		return nil
	}
	return ErrPromptUnsupported
}
