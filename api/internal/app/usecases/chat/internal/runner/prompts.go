package runner

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
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

const maxReactPromptBytes = 64 * 1024

func (rs *Runners) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID string,
) (domain.AgentPromptSubmission, error) {
	clientRequestID, err := normalisePromptRequest(text, clientRequestID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	textHash := agentjournal.PromptTextHash(text)

	defer rs.spawns.Lock(chatID)()

	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: chat: %w", err)
	}
	chatsDir, err := rs.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: chats dir: %w", err)
	}
	journalDir := rs.prompts.Dir(chatsDir, chat.ID)
	if err := rs.ReconcilePendingPromptFromLedger(ctx, chat); err != nil {
		slog.ErrorContext(ctx, "agent: reconcile React prompt before submission (best-effort)",
			"chat_id", chat.ID, "err", err)
	}

	result, done, err := rs.replayPriorAttempt(ctx, chat, journalDir, clientRequestID, textHash)
	if done {
		return result, err
	}

	live, descriptor, worktree, err := rs.promptTarget(ctx, chat)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	if err := rs.requirePromptIdle(ctx, chatID, live.ID); err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	if submission, handled, pushErr := rs.submitPromptOverAPI(
		ctx, chat, journalDir, clientRequestID, textHash, live, worktree, text,
	); handled {
		return submission, pushErr
	}

	delivery, err := rs.resolvePromptDelivery(ctx, chat.ID, live, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	replacementRunnerID := uuid.NewString()
	prior, existingAttempt, err := rs.prompts.Begin(
		journalDir, clientRequestID, textHash, live.ProviderID, live.ID, replacementRunnerID, time.Now(),
	)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf(
			"agent: submit prompt: begin durable dispatch: %w", promptJournalError(err),
		)
	}
	if existingAttempt {
		result, done, classifyErr := rs.classifyPriorAttempt(
			ctx, chat, journalDir, clientRequestID, prior,
		)
		if done {
			return result, classifyErr
		}
		return domain.AgentPromptSubmission{}, ErrPromptOutcomeUnknown
	}
	if err := rs.displaceForPrompt(ctx, chatID, journalDir, clientRequestID, live.ID); err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	runnerID, err := rs.spawnRunner(
		ctx, chatID, chat.WorkspaceID, live.ProviderID, replacementRunnerID,
		delivery.resumeSteps, delivery.promptSteps, delivery.conversation, 0,
		delivery.contextResuming, delivery.launchSessionID, false, text,
	)
	if err != nil {
		if errors.Is(err, engineterminal.ErrCommandNotFound) {
			_ = rs.prompts.MarkFailedDispatch(journalDir, clientRequestID, time.Now())
			return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: replacement spawn: %w", err)
		}
		return domain.AgentPromptSubmission{}, rs.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "replacement spawn", err,
		)
	}
	return rs.commitPromptSpawn(ctx, journalDir, clientRequestID, textHash, runnerID)
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

func (rs *Runners) replayPriorAttempt(
	ctx context.Context,
	chat domain.Chat,
	journalDir, clientRequestID, textHash string,
) (domain.AgentPromptSubmission, bool, error) {
	existing, found, err := rs.prompts.Lookup(journalDir, clientRequestID, textHash)
	err = promptJournalError(err)
	if errors.Is(err, ErrPromptRequestIDConflict) {
		return domain.AgentPromptSubmission{}, true, err
	}
	if err != nil {
		return domain.AgentPromptSubmission{}, true, rs.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "idempotency lookup", err,
		)
	}
	if !found {
		return domain.AgentPromptSubmission{}, false, nil
	}
	return rs.classifyPriorAttempt(ctx, chat, journalDir, clientRequestID, existing)
}

// submitPromptOverAPI delivers text over live's connection when it has one —
// the mixed-transport case, where a message needs no restart at all: the same
// connection applyAPITransport opened at spawn carries every message the
// conversation ever sends, first or hundredth. handled=false means live has no
// live api connection (every hooks-only provider, and a mixed-transport one
// whose serve process never came up); the caller falls back to the
// restart_tui path unchanged.
func (rs *Runners) submitPromptOverAPI(
	ctx context.Context,
	chat domain.Chat,
	journalDir, clientRequestID, textHash string,
	live engineagents.Runner,
	worktree, text string,
) (domain.AgentPromptSubmission, bool, error) {
	if _, ok := rs.apiConns.get(live.ID); !ok {
		return domain.AgentPromptSubmission{}, false, nil
	}

	prior, existingAttempt, err := rs.prompts.Begin(
		journalDir, clientRequestID, textHash, live.ProviderID, live.ID, live.ID, time.Now(),
	)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf(
			"agent: submit prompt: begin durable dispatch: %w", promptJournalError(err),
		)
	}
	if existingAttempt {
		result, done, classifyErr := rs.classifyPriorAttempt(ctx, chat, journalDir, clientRequestID, prior)
		if done {
			return result, true, classifyErr
		}
		return domain.AgentPromptSubmission{}, true, ErrPromptOutcomeUnknown
	}

	_, _, pushErr := rs.pushPromptOverAPI(ctx, live.ID, live.CurrentSession, worktree, text)
	if pushErr != nil {
		return domain.AgentPromptSubmission{}, true, rs.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "api push", pushErr,
		)
	}
	submission, err := rs.commitPromptSpawn(ctx, journalDir, clientRequestID, textHash, live.ID)
	return submission, true, err
}

func (rs *Runners) promptTarget(
	ctx context.Context,
	chat domain.Chat,
) (engineagents.Runner, engineagents.Agent, string, error) {
	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chat.ID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return engineagents.Runner{}, nil, "", ErrPromptSessionUnavailable
	}
	if err != nil {
		return engineagents.Runner{}, nil, "", fmt.Errorf("agent: submit prompt: live runner: %w", err)
	}
	crowbarHome, _, _, worktree, err := rs.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return engineagents.Runner{}, nil, "", fmt.Errorf("agent: submit prompt: worktree dir: %w", err)
	}
	descriptor, err := rs.agents.Get(ctx, crowbarHome, live.ProviderID)
	if err != nil {
		return engineagents.Runner{}, nil, "", fmt.Errorf("agent: submit prompt: resolve descriptor: %w", err)
	}
	return live, descriptor, worktree, nil
}

func (rs *Runners) displaceForPrompt(
	ctx context.Context,
	chatID, journalDir, clientRequestID, liveRunnerID string,
) error {
	unlockTurnStart := rs.turnStarts.Lock(chatID)
	defer unlockTurnStart()

	if err := rs.requirePromptIdle(ctx, chatID, liveRunnerID); err != nil {
		_ = rs.prompts.MarkFailedDispatch(journalDir, clientRequestID, time.Now())
		return err
	}

	if err := rs.quitOutgoingCLI(ctx, chatID); err != nil {
		_ = rs.prompts.MarkFailedDispatch(journalDir, clientRequestID, time.Now())
		return err
	}
	return nil
}

func (rs *Runners) commitPromptSpawn(
	ctx context.Context,
	journalDir, clientRequestID, textHash, runnerID string,
) (domain.AgentPromptSubmission, error) {
	spawned, err := rs.runnerStore.Get(ctx, runnerID)
	if err != nil {
		return domain.AgentPromptSubmission{}, rs.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "read replacement runner", err,
		)
	}
	result := domain.AgentPromptSubmission{
		RunnerID:          runnerID,
		TerminalSessionID: spawned.TerminalSession,
	}
	if result.TerminalSessionID == "" {
		return domain.AgentPromptSubmission{}, rs.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "replacement terminal identity is missing", nil,
		)
	}
	committed, err := rs.prompts.MarkSpawned(
		journalDir, clientRequestID, textHash, result.RunnerID, result.TerminalSessionID, time.Now(),
	)
	if err != nil {
		return domain.AgentPromptSubmission{}, rs.markPromptOutcomeUncertain(
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

func (rs *Runners) markPromptOutcomeUncertain(
	ctx context.Context,
	journalDir, clientRequestID, stage string,
	cause error,
) error {
	if err := rs.prompts.MarkUncertain(journalDir, clientRequestID, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: persist uncertain React prompt outcome",
			"stage", stage, "client_request_id", clientRequestID, "err", err)
	}
	return uncertainPromptOutcome(ctx, stage, cause)
}

func (rs *Runners) requirePromptIdle(ctx context.Context, chatID, runnerID string) error {
	current, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return ErrPromptSessionUnavailable
	}
	if err != nil {
		return fmt.Errorf("agent: submit prompt: refresh live runner: %w", err)
	}
	if current.ID != runnerID {
		return ErrPromptBusy
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: refresh chat: %w", err)
	}

	if chat.Working || len(rs.inflightTurns.Inflight(chatID)) > 0 {
		return ErrPromptBusy
	}
	return nil
}

func (rs *Runners) requireNoPendingPromptDelivery(ctx context.Context, chat domain.Chat) error {
	if err := rs.ReconcilePendingPromptFromLedger(ctx, chat); err != nil {
		return fmt.Errorf("agent: prompt delivery guard: reconcile ledger evidence: %w", err)
	}
	chatsDir, err := rs.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: prompt delivery guard: chats dir: %w", err)
	}
	pending, err := rs.prompts.HasPendingDelivery(rs.prompts.Dir(chatsDir, chat.ID))
	if err != nil {
		return fmt.Errorf("agent: prompt delivery guard: inspect journal: %w", err)
	}
	if pending {
		return ErrPromptBusy
	}
	return nil
}

func (rs *Runners) reconcilePromptJournalsOnBoot(ctx context.Context) error {
	chats, err := rs.chats.ListChats(ctx)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile prompt journals: list chats: %w", err)
	}
	dirs := make(map[string]string)
	now := time.Now()
	for _, chat := range chats {
		chatsDir, ok := dirs[chat.WorkspaceID]
		if !ok {
			chatsDir, err = rs.ws.AgentChatsDir(ctx, chat.WorkspaceID)
			if err != nil {
				return fmt.Errorf("agent: boot reconcile prompt journals: chats dir for %s: %w", chat.ID, err)
			}
			dirs[chat.WorkspaceID] = chatsDir
		}
		dir := rs.prompts.Dir(chatsDir, chat.ID)
		if err := rs.prompts.RecoverOrphanedDispatches(dir, now); err != nil {
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

// apiOwnsResume reports whether resuming d's session is applyAPITransport's
// job (codex.yaml's prompt.resume: thread/resume), never the redundant
// hooks-only PTY's (see codex.yaml's own comment on subagent_pre for why that
// PTY exists at all). Handing that SAME session id to the PTY too — as a
// native `resume {id}` argv, or as the resume context pointer, which for a
// provider whose only resume channel is a user message IS a prompt the PTY
// will act on — makes it a SECOND writer on a thread the api connection
// already holds. codex enforces one writer per thread (a thread-writer-lock
// file, confirmed on disk — the same constraint the header comment documents
// for --remote attach): confirmed live, the native `resume {id}` case exits
// immediately (exit code 1), onRunnerExit reconciles the whole runner as
// dead, and the switch that looked like it succeeded silently reverts.
// Confirmed live also: WITHOUT the id but WITH the pointer, the PTY no longer
// collides — it just answers the pointer itself, as its own genuine first
// turn, and that reply lands on this chat as a second, disconnected "codex"
// conversation the api connection knows nothing about. So both must be
// withheld from this PTY, not just the id: with neither, it starts idle (like
// today's already-accepted fresh-spawn "own unrelated conversation" gap) and
// never announces anything for resumableConversation to trip over.
//
// The api connection resuming silently, with no gap handed to it either, is
// the SAME known gap this leaves in place: codex.yaml declares an inject: at:
// context step (thread/inject_items) for exactly this, and nothing calls it
// yet. Recorded, not silently papered over — see the mixed-transport design
// spec.
func apiOwnsResume(d engineagents.Agent) bool {
	return d.TransportFor("prompt") == "api" && !d.Capabilities().Hotswap
}

func nativeResumeSteps(d engineagents.Agent, sessionID string) []engineagents.InjectStep {
	if apiOwnsResume(d) {
		return nil
	}
	return resumeInjectionSteps(d, sessionID)
}

func promptSubmission(record agentjournal.PromptRequest) domain.AgentPromptSubmission {
	return domain.AgentPromptSubmission{
		RunnerID:          record.RunnerID,
		TerminalSessionID: record.TerminalSessionID,
	}
}

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
	launched := engineagents.Selection{Model: live.LaunchModel, Effort: live.LaunchEffort}
	if descriptor.SelectionRestart(launched, desired) {
		return nil
	}
	return ErrPromptUnsupported
}
