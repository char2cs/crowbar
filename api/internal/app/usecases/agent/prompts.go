package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/chatlog"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

const (
	defaultMessagePageLimit = 100
	maxMessagePageLimit     = 200
	maxReactPromptBytes     = 64 * 1024
)

// ReadMessages returns a bounded chronological page from Crowbar's hook-derived
// ledger. It never reads a provider transcript or provider-owned file.
func (u *Usecase) ReadMessages(
	ctx context.Context,
	chatID string,
	after, before, limit int,
) (chatlog.Page, error) {
	if after < 0 || before < 0 || (after > 0 && before > 0) {
		return chatlog.Page{}, fmt.Errorf("agent: read messages: invalid cursor: %w", apperr.ErrInvalidArgument)
	}
	if limit == 0 {
		limit = defaultMessagePageLimit
	}
	if limit < 1 || limit > maxMessagePageLimit {
		return chatlog.Page{}, fmt.Errorf("agent: read messages: limit must be between 1 and %d: %w", maxMessagePageLimit, apperr.ErrInvalidArgument)
	}
	if _, err := u.chats.GetChat(ctx, chatID); err != nil {
		return chatlog.Page{}, fmt.Errorf("agent: read messages: chat: %w", err)
	}
	page, err := u.chatPage(ctx, chatID, after, before, limit)
	if err != nil {
		return chatlog.Page{}, fmt.Errorf("agent: read messages: %w", err)
	}
	return page, nil
}

// SubmitPrompt restarts a chat's current provider as another normal interactive
// TUI and supplies text through the descriptor's documented positional prompt
// mapping. Hooks remain the only ledger writer.
func (u *Usecase) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID string,
) (dto.PromptSubmissionDTO, error) {
	if strings.TrimSpace(text) == "" {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: text is required: %w", apperr.ErrInvalidArgument)
	}
	if len([]byte(text)) > maxReactPromptBytes {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: text exceeds %d bytes: %w", maxReactPromptBytes, apperr.ErrInvalidArgument)
	}
	// POSIX argv strings cannot contain NUL. Reject it before the durable intent
	// and before touching the live TUI, so this is a definitive client error rather
	// than an outcome-unknown dispatch after the outgoing process was torn down.
	if strings.IndexByte(text, 0) >= 0 {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: text contains an unsupported NUL byte: %w", apperr.ErrInvalidArgument)
	}
	requestUUID, err := uuid.Parse(clientRequestID)
	if err != nil {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: clientRequestId must be a UUID: %w", apperr.ErrInvalidArgument)
	}
	clientRequestID = requestUUID.String()
	textHash := promptTextHash(text)

	defer u.spawns.lock(chatID)()

	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: chat: %w", err)
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: chats dir: %w", err)
	}
	journalDir := promptJournalDir(filepath.Join(chatsDir, chat.ID))
	if err := u.reconcilePendingPromptFromLedger(ctx, chat); err != nil {
		slog.ErrorContext(ctx, "agent: reconcile React prompt before submission (best-effort)",
			"chat_id", chat.ID, "err", err)
	}

	// Idempotency precedes current-state checks. A retry after a lost HTTP response
	// must receive the original successful spawn even though that spawn has since
	// made the chat busy.
	existing, found, err := u.prompts.lookup(journalDir, clientRequestID, textHash)
	if err != nil {
		if errors.Is(err, ErrPromptRequestIDConflict) {
			return dto.PromptSubmissionDTO{}, err
		}
		return dto.PromptSubmissionDTO{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "idempotency lookup", err,
		)
	}
	if found {
		result, done, err := u.classifyPriorAttempt(ctx, chat, journalDir, clientRequestID, existing)
		if done {
			return result, err
		}
		// A failed record is a failure proven to precede replacement process
		// startup and safely falls through to a same-id retry.
	}

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return dto.PromptSubmissionDTO{}, ErrPromptSessionUnavailable
	}
	if err != nil {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: live runner: %w", err)
	}
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: worktree dir: %w", err)
	}
	descriptor, err := u.agents.Get(ctx, crowbarHome, live.ProviderID)
	if err != nil {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: resolve descriptor: %w", err)
	}

	resuming := false
	nativeSessionID := live.CurrentSession
	// A TUI launched with an explicit native resume target is continuing that
	// conversation even when its session_start is still in flight, or when every
	// existing turn predates this runner's bind timestamp. The durable
	// launch identity is the authoritative answer for that case.
	if live.LaunchSessionID != "" &&
		(live.CurrentSession == "" || live.CurrentSession == live.LaunchSessionID) {
		resuming = true
		nativeSessionID = live.LaunchSessionID
	} else if live.CurrentSession != "" && live.CurrentSessionResumable {
		// A native TUI /resume is learned from the same append-only session
		// history as Crowbar's own ResumeChat. Its existing turns legitimately
		// predate this binding, so the persisted known-session fact wins over the
		// timestamp heuristic without any provider-specific hook vocabulary.
		resuming = true
	} else if live.CurrentSession != "" && !live.CurrentSessionSince.IsZero() {
		resuming, err = u.activity.HasTurnAtOrAfter(ctx, chatID, live.ProviderID, live.CurrentSessionSince)
		if err != nil {
			return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: inspect current conversation: %w", err)
		}
	}
	promptSteps, err := descriptor.PromptSteps(resuming)
	if err != nil {
		if errors.Is(err, engineagents.ErrPromptSubmitUnsupported) {
			return dto.PromptSubmissionDTO{}, ErrPromptUnsupported
		}
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: render prompt mapping: %w", err)
	}
	var resumeSteps []engineagents.InjectStep
	launchSessionID := ""
	if resuming {
		resumeSteps = resumeInjectionSteps(descriptor, nativeSessionID)
		if len(resumeSteps) == 0 {
			return dto.PromptSubmissionDTO{}, ErrPromptUnsupported
		}
		launchSessionID = nativeSessionID
	}

	// Preflight can perform descriptor and record IO. Re-read placement and busy
	// state only after it, immediately before durable dispatch, so a native turn
	// or conversation move that happened during preflight wins with 409 rather
	// than being killed.
	if err := u.requirePromptIdle(ctx, chatID, live.ID); err != nil {
		return dto.PromptSubmissionDTO{}, err
	}

	replacementRunnerID := uuid.NewString()
	prior, existingAttempt, err := u.prompts.begin(
		journalDir, clientRequestID, textHash, live.ProviderID, live.ID, replacementRunnerID, time.Now(),
	)
	if err != nil {
		return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: begin durable dispatch: %w", err)
	}
	if existingAttempt {
		// UNREACHABLE while this function holds the chat gate: two submissions of
		// one request id are serialised, so the second always meets a completed
		// record at the lookup above and returns there. begin can therefore only
		// report an existing attempt if that serialisation is ever relaxed.
		//
		// It is handled rather than ignored anyway, and handled by the SAME
		// classifier the lookup uses. An empty branch here would mean that the day
		// the gate changes, this path silently spawns a second CLI and delivers the
		// prompt twice — the one outcome the at-most-once journal exists to prevent.
		result, done, classifyErr := u.classifyPriorAttempt(
			ctx, chat, journalDir, clientRequestID, prior,
		)
		if done {
			return result, classifyErr
		}
		return dto.PromptSubmissionDTO{}, ErrPromptOutcomeUnknown
	}
	// begin fsyncs the at-most-once intent. The turn-start interlock makes the
	// following final idle check and displacement one atomic section with respect
	// to native/user_prompt hooks: a prompt either starts its turn first and this
	// aborts, or displacement happens first and the stale outgoing hook is dropped.
	unlockTurnStart := u.turnStarts.lock(chatID)
	if err := u.requirePromptIdle(ctx, chatID, live.ID); err != nil {
		unlockTurnStart()
		_ = u.prompts.markFailedDispatch(journalDir, clientRequestID, time.Now())
		return dto.PromptSubmissionDTO{}, err
	}

	// No handoff is assembled: this is the same provider continuing its own native
	// conversation. Fresh gets normal initial context; resume gets its native id.
	if err := u.quitOutgoingCLI(ctx, chatID); err != nil {
		unlockTurnStart()
		// quitOutgoingCLI surfaces terminate failure before displacement, so no
		// replacement was attempted. Marking failed makes same-id retry safe.
		_ = u.prompts.markFailedDispatch(journalDir, clientRequestID, time.Now())
		return dto.PromptSubmissionDTO{}, err
	}
	unlockTurnStart()

	runnerID, err := u.spawnRunner(
		ctx, chatID, chat.WorkspaceID, live.ProviderID, replacementRunnerID,
		resumeSteps, promptSteps, "", 0, resuming, launchSessionID, false, text,
	)
	if err != nil {
		// Once spawnRunner is entered, a process may have existed even when its
		// persistence failed. Leave dispatching durable: retry is outcome-unknown,
		// never a silent duplicate.
		if errors.Is(err, engineterminal.ErrCommandNotFound) {
			// Command-not-found is classified before a process exists. Retrying
			// after installation with the same id is therefore safe.
			_ = u.prompts.markFailedDispatch(journalDir, clientRequestID, time.Now())
			return dto.PromptSubmissionDTO{}, fmt.Errorf("agent: submit prompt: replacement spawn: %w", err)
		}
		return dto.PromptSubmissionDTO{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "replacement spawn", err,
		)
	}
	spawned, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		return dto.PromptSubmissionDTO{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "read replacement runner", err,
		)
	}
	result := dto.PromptSubmissionDTO{
		RunnerID:          runnerID,
		TerminalSessionID: spawned.TerminalSession,
	}
	if result.TerminalSessionID == "" {
		return dto.PromptSubmissionDTO{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "replacement terminal identity is missing", nil,
		)
	}
	committed, err := u.prompts.markSpawned(journalDir, clientRequestID, textHash, result, time.Now())
	if err != nil {
		return dto.PromptSubmissionDTO{}, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "persist replacement identity", err,
		)
	}
	if committed.State == promptStateUncertain {
		return dto.PromptSubmissionDTO{}, ErrPromptOutcomeUnknown
	}
	return result, nil
}

// uncertainPromptOutcome deliberately keeps the internal failure out of the
// public error chain. Several repository sentinels map to 404/500 ahead of the
// conflict category, but once the replacement CLI may have received the prompt
// the only safe client contract is request_outcome_uncertain/409. The cause is
// retained in structured daemon logs for diagnosis.
func uncertainPromptOutcome(ctx context.Context, stage string, cause error) error {
	attrs := []any{"stage", stage}
	if cause != nil {
		attrs = append(attrs, "err", cause)
	}
	slog.ErrorContext(ctx, "agent: React prompt delivery outcome is uncertain", attrs...)
	return ErrPromptOutcomeUnknown
}

func (u *Usecase) markPromptOutcomeUncertain(
	ctx context.Context,
	journalDir, clientRequestID, stage string,
	cause error,
) error {
	if err := u.prompts.markUncertain(journalDir, clientRequestID, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: persist uncertain React prompt outcome",
			"stage", stage, "client_request_id", clientRequestID, "err", err)
	}
	return uncertainPromptOutcome(ctx, stage, cause)
}

func (u *Usecase) requirePromptIdle(ctx context.Context, chatID, runnerID string) error {
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
	// Folded async work comes from the aggregate; turns.inflight closes the
	// projection-lag window immediately after a user_prompt hook.
	if chat.Working || len(u.turns.inflight(chatID)) > 0 {
		return ErrPromptBusy
	}
	return nil
}

// requireNoPendingPromptDelivery protects the replacement TUI from destructive
// actions issued by another browser window during the short spawn-to-user_prompt
// acknowledgement window. Callers hold the same per-chat spawn gate as prompt
// submission, so the check and any following teardown cannot race another begin.
func (u *Usecase) requireNoPendingPromptDelivery(ctx context.Context, chat domain.AgentChat) error {
	if err := u.reconcilePendingPromptFromLedger(ctx, chat); err != nil {
		return fmt.Errorf("agent: prompt delivery guard: reconcile ledger evidence: %w", err)
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("agent: prompt delivery guard: chats dir: %w", err)
	}
	pending, err := u.prompts.hasPendingDelivery(promptJournalDir(filepath.Join(chatsDir, chat.ID)))
	if err != nil {
		return fmt.Errorf("agent: prompt delivery guard: inspect journal: %w", err)
	}
	if pending {
		return ErrPromptBusy
	}
	return nil
}

// reconcilePromptJournalsOnBoot releases durable dispatch intents whose process
// identity was never committed before the daemon died. They remain visible as
// uncertain to same-id retries, but cease acting like a live delivery barrier.
func (u *Usecase) reconcilePromptJournalsOnBoot(ctx context.Context) error {
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
		dir := promptJournalDir(filepath.Join(chatsDir, chat.ID))
		if err := u.prompts.recoverOrphanedDispatches(dir, now); err != nil {
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

// classifyPriorAttempt answers what a request id's existing journal record means
// for a submission that is trying to use it again.
//
// done=false means only one thing: the record is a FAILURE proven to precede any
// replacement process, which is the single state a same-id retry may safely fall
// through. Every other state is terminal for this submission, because a prompt
// whose outcome is unknown must never be delivered a second time on a guess.
func (u *Usecase) classifyPriorAttempt(
	ctx context.Context,
	chat domain.AgentChat,
	journalDir, clientRequestID string,
	existing promptRequestRecord,
) (dto.PromptSubmissionDTO, bool, error) {
	if existing.RunnerID != "" && existing.TerminalSessionID != "" &&
		(existing.State == promptStateSpawned || existing.State == promptStateAccepted) {
		return existing.result(), true, nil
	}
	if existing.State == promptStateDispatching ||
		existing.State == promptStateSpawned ||
		existing.State == promptStateUncertain {
		accepted, err := u.promptRecordAccepted(ctx, chat, existing)
		if err != nil {
			return dto.PromptSubmissionDTO{}, true, u.markPromptOutcomeUncertain(
				ctx, journalDir, clientRequestID, "recover prior delivery", err,
			)
		}
		if accepted {
			if _, err := u.prompts.markAcceptedByRequest(journalDir, clientRequestID, time.Now()); err != nil {
				slog.ErrorContext(ctx, "agent: submit prompt: persist recovered acceptance",
					"chat_id", chat.ID, "client_request_id", clientRequestID, "err", err)
			}
			return dto.PromptSubmissionDTO{}, true, ErrPromptAlreadyAccepted
		}
		return dto.PromptSubmissionDTO{}, true, ErrPromptOutcomeUnknown
	}
	if existing.State == promptStateAccepted {
		return dto.PromptSubmissionDTO{}, true, ErrPromptAlreadyAccepted
	}
	return dto.PromptSubmissionDTO{}, false, nil
}

func (u *Usecase) promptRecordAccepted(
	ctx context.Context,
	chat domain.AgentChat,
	record promptRequestRecord,
) (bool, error) {
	turns, err := u.chatTurns(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: recover prompt request: turns: %w", err)
	}
	for _, turn := range turns {
		if turn.Role != "user" || turn.Provider != record.ProviderID || turn.At.Before(record.CreatedAt) {
			continue
		}
		if record.RunnerID != "" {
			if turn.RunnerID != record.RunnerID {
				continue
			}
		} else if turn.RunnerID == "" || turn.RunnerID == record.OutgoingRunnerID {
			// In the process-commit crash gap the replacement id was not yet
			// journaled. A hook from any runner except the outgoing one is
			// positive delivery evidence; legacy rows with no runner attribution
			// are intentionally insufficient for automatic recovery.
			continue
		}
		if promptTextHash(turn.Text) == record.TextHash {
			return true, nil
		}
	}
	return false, nil
}

// reconcilePendingPromptFromLedger repairs the crash/IO gap where the
// authoritative attributed user turn was committed but the request journal did
// not advance. It is safe to call repeatedly: ledger evidence is append-only and
// markAcceptedByRequest is an idempotent state assignment.
func (u *Usecase) reconcilePendingPromptFromLedger(
	ctx context.Context,
	chat domain.AgentChat,
) error {
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("reconcile prompt acceptance: chats dir: %w", err)
	}
	dir := promptJournalDir(filepath.Join(chatsDir, chat.ID))
	record, found, err := u.prompts.activeDelivery(dir)
	if err != nil || !found {
		return err
	}
	accepted, err := u.promptRecordAccepted(ctx, chat, record)
	if err != nil || !accepted {
		return err
	}
	if _, err := u.prompts.markAcceptedByRequest(dir, record.RequestID, time.Now()); err != nil {
		return fmt.Errorf("reconcile prompt acceptance: persist: %w", err)
	}
	return nil
}

func (u *Usecase) confirmPromptAccepted(
	ctx context.Context,
	chat domain.AgentChat,
	runner domain.AgentRunner,
	text string,
) error {
	chatsDir, err := u.ws.AgentChatsDir(ctx, chat.WorkspaceID)
	if err != nil {
		return fmt.Errorf("prompt journal chats dir: %w", err)
	}
	dir := promptJournalDir(filepath.Join(chatsDir, chat.ID))
	return u.prompts.confirmAccepted(
		dir, runner.ID, runner.ProviderID, promptTextHash(text), time.Now(),
	)
}

// reconcilePromptRunnerDeparture closes a pending dispatch when its replacement
// runner can no longer emit a user_prompt hook. A matching ledger user turn wins
// and confirms delivery; only a runner with no such hook is marked failed and
// eligible for same-id retry.
func (u *Usecase) reconcilePromptRunnerDeparture(
	ctx context.Context,
	runner domain.AgentRunner,
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
	dir := promptJournalDir(filepath.Join(chatsDir, chat.ID))
	record, found, err := u.prompts.activeForRunner(dir, runner.ID, runner.ProviderID)
	if err != nil || !found {
		return
	}
	accepted, err := u.promptRecordAccepted(ctx, chat, record)
	if err != nil {
		return
	}
	if accepted {
		_, _ = u.prompts.markAcceptedByRequest(dir, record.RequestID, time.Now())
		return
	}
	// A process exit can race an already-issued hook HTTP request. Without a
	// matching attributed ledger turn the honest state is uncertain, not safely
	// failed; same-id automatic retry must never duplicate that late acceptance.
	_ = u.prompts.markUncertain(dir, record.RequestID, time.Now())
}
