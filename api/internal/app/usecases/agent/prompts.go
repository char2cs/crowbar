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
	clientRequestID, err := normalisePromptRequest(text, clientRequestID)
	if err != nil {
		return dto.PromptSubmissionDTO{}, err
	}
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

	result, done, err := u.replayPriorAttempt(ctx, chat, journalDir, clientRequestID, textHash)
	if done {
		return result, err
	}

	live, descriptor, err := u.promptTarget(ctx, chat)
	if err != nil {
		return dto.PromptSubmissionDTO{}, err
	}
	delivery, err := u.resolvePromptDelivery(ctx, chat.ID, live, descriptor)
	if err != nil {
		return dto.PromptSubmissionDTO{}, err
	}

	// Preflight can perform descriptor and record IO. Re-read placement and busy
	// state only after it, immediately before durable dispatch, so a native turn
	// or conversation move that happened during preflight wins with 409 rather
	// than being killed.
	if err := u.requirePromptIdle(ctx, chatID, live.ID); err != nil {
		return dto.PromptSubmissionDTO{}, err
	}

	// The runner this dispatch is ATTRIBUTED to from its first fsync. A restart
	// attributes to the process it is about to create; a rewake attributes to the
	// live one, because that is the process that will emit the prompt hook — which
	// is what lets a daemon that dies mid-dispatch resolve the outcome from the
	// conversation record instead of reporting it unknown.
	journaledRunnerID := uuid.NewString()
	if delivery.rewake {
		journaledRunnerID = live.ID
	}
	prior, existingAttempt, err := u.prompts.begin(
		journalDir, clientRequestID, textHash, live.ProviderID, live.ID, journaledRunnerID, time.Now(),
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
	// The optimisation, and the ONE place it is allowed to be one: it is tried, and
	// a failure to place the message costs nothing but the floor below. Everything
	// after this line is the delivery Crowbar has always performed.
	if delivery.rewake {
		result, done, deliverErr := u.deliverToLiveSession(
			ctx, journalDir, clientRequestID, textHash, chatID, live, text,
		)
		if done {
			return result, deliverErr
		}
		// done=false is PROOF that no collector took these bytes (see
		// rewakeDesk.deliver), not a hope. So the message is still undelivered, and
		// the durable intent written above stands for the restart that follows.
		slog.InfoContext(ctx, "agent: submit prompt: no rewake collector for this chat; delivering by restart",
			"chat_id", chatID, "runner_id", live.ID, "provider", live.ProviderID)
	}

	return u.deliverByReplacement(
		ctx, chat, journalDir, clientRequestID, textHash, journaledRunnerID, text, live, delivery,
	)
}

// deliverByReplacement is the portable floor: the CLI is replaced by another
// ordinary interactive one carrying the message as its final argv element.
//
// It is reached in two ways and behaves identically in both — a provider that
// declares restart_tui, and a rewake provider whose message found no collector.
// The second case is why the durable intent may have to be re-attributed first:
// it was journaled against the live runner, which is about to be quit.
func (u *Usecase) deliverByReplacement(
	ctx context.Context,
	chat domain.AgentChat,
	journalDir, clientRequestID, textHash, journaledRunnerID, text string,
	live domain.AgentRunner,
	delivery promptDelivery,
) (dto.PromptSubmissionDTO, error) {
	// A rewake that fell through never created a process, so the replacement is a
	// new identity — and the journal has to learn it BEFORE the displacement below
	// kills the runner it is currently attributed to. See repointDispatch.
	replacementRunnerID := journaledRunnerID
	if delivery.rewake {
		replacementRunnerID = uuid.NewString()
		if err := u.prompts.repointDispatch(
			journalDir, clientRequestID, replacementRunnerID, time.Now(),
		); err != nil {
			// Nothing destructive has happened and nothing was delivered, so this is
			// a FAILED dispatch the same id may retry — not an unknown outcome.
			_ = u.prompts.markFailedDispatch(journalDir, clientRequestID, time.Now())
			return dto.PromptSubmissionDTO{}, fmt.Errorf(
				"agent: submit prompt: re-attribute dispatch for restart fallback: %w", err)
		}
	}
	if err := u.displaceForPrompt(ctx, chat.ID, journalDir, clientRequestID, live.ID); err != nil {
		return dto.PromptSubmissionDTO{}, err
	}

	runnerID, err := u.spawnRunner(
		ctx, chat.ID, chat.WorkspaceID, live.ProviderID, replacementRunnerID,
		delivery.resumeSteps, delivery.promptSteps, "", 0,
		delivery.resuming, delivery.launchSessionID, false, text,
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
	return u.commitPromptSpawn(ctx, journalDir, clientRequestID, textHash, runnerID)
}

// normalisePromptRequest rejects a message that cannot become argv, and pins the
// request id to its canonical form.
//
// Every check here runs BEFORE the durable intent and before the live TUI is
// touched, so each one is a definitive client error rather than an
// outcome-unknown dispatch against a process that has already been torn down.
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
	// POSIX argv strings cannot contain NUL.
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

// replayPriorAttempt answers what this request id has already done.
//
// Idempotency precedes every current-state check: a retry after a lost HTTP
// response must receive the original successful spawn even though that spawn has
// since made the chat busy. done=false means the id is free to proceed — either
// it is unknown, or its record is a FAILURE proven to precede any replacement
// process, which is the one state a same-id retry may safely fall through.
func (u *Usecase) replayPriorAttempt(
	ctx context.Context,
	chat domain.AgentChat,
	journalDir, clientRequestID, textHash string,
) (dto.PromptSubmissionDTO, bool, error) {
	existing, found, err := u.prompts.lookup(journalDir, clientRequestID, textHash)
	if errors.Is(err, ErrPromptRequestIDConflict) {
		return dto.PromptSubmissionDTO{}, true, err
	}
	if err != nil {
		return dto.PromptSubmissionDTO{}, true, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "idempotency lookup", err,
		)
	}
	if !found {
		return dto.PromptSubmissionDTO{}, false, nil
	}
	return u.classifyPriorAttempt(ctx, chat, journalDir, clientRequestID, existing)
}

// promptTarget resolves the process this message is for and the descriptor that
// knows how to reach it.
func (u *Usecase) promptTarget(
	ctx context.Context,
	chat domain.AgentChat,
) (domain.AgentRunner, engineagents.Agent, error) {
	live, err := u.runners.LiveRunnerForChat(ctx, chat.ID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return domain.AgentRunner{}, nil, ErrPromptSessionUnavailable
	}
	if err != nil {
		return domain.AgentRunner{}, nil, fmt.Errorf("agent: submit prompt: live runner: %w", err)
	}
	crowbarHome, _, _, _, err := u.ws.WorktreeDir(ctx, chat.WorkspaceID)
	if err != nil {
		return domain.AgentRunner{}, nil, fmt.Errorf("agent: submit prompt: worktree dir: %w", err)
	}
	descriptor, err := u.agents.Get(ctx, crowbarHome, live.ProviderID)
	if err != nil {
		return domain.AgentRunner{}, nil, fmt.Errorf("agent: submit prompt: resolve descriptor: %w", err)
	}
	return live, descriptor, nil
}

// displaceForPrompt takes the outgoing CLI off the chat so the replacement can
// be spawned onto it.
//
// begin has already fsynced the at-most-once intent by the time this runs, and
// the turn-start interlock makes the final idle check and the displacement ONE
// atomic section with respect to native user_prompt hooks: a native prompt either
// starts its turn first and this aborts, or displacement happens first and the
// stale outgoing hook is dropped. Either failure marks the dispatch FAILED — both
// are proven to precede any replacement process, so a same-id retry is safe.
func (u *Usecase) displaceForPrompt(
	ctx context.Context,
	chatID, journalDir, clientRequestID, liveRunnerID string,
) error {
	unlockTurnStart := u.turnStarts.lock(chatID)
	defer unlockTurnStart()

	if err := u.requirePromptIdle(ctx, chatID, liveRunnerID); err != nil {
		_ = u.prompts.markFailedDispatch(journalDir, clientRequestID, time.Now())
		return err
	}
	// No handoff is assembled: this is the same provider continuing its own native
	// conversation. Fresh gets normal initial context; resume gets its native id.
	if err := u.quitOutgoingCLI(ctx, chatID); err != nil {
		// quitOutgoingCLI surfaces terminate failure before displacement, so no
		// replacement was attempted.
		_ = u.prompts.markFailedDispatch(journalDir, clientRequestID, time.Now())
		return err
	}
	return nil
}

// commitPromptSpawn records the replacement process's identity, which is what
// turns a durable dispatch intent into a completed one.
//
// Every failure past this point is outcome-UNCERTAIN rather than failed: the
// replacement may already hold the prompt, so a retry must never be allowed to
// deliver it twice.
func (u *Usecase) commitPromptSpawn(
	ctx context.Context,
	journalDir, clientRequestID, textHash, runnerID string,
) (dto.PromptSubmissionDTO, error) {
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
		return u.recoverPriorDelivery(ctx, chat, journalDir, clientRequestID, existing)
	}
	if existing.State == promptStateAccepted {
		return dto.PromptSubmissionDTO{}, true, ErrPromptAlreadyAccepted
	}
	return dto.PromptSubmissionDTO{}, false, nil
}

// recoverPriorDelivery decides an attempt whose journal record stopped short of
// a committed outcome, by asking the conversation record whether the prompt was
// in fact delivered.
//
// The answer is never a re-delivery: the worst case is reporting an unknown
// outcome for a prompt that did arrive, which the user can see in the chat. The
// opposite mistake would send it twice.
func (u *Usecase) recoverPriorDelivery(
	ctx context.Context,
	chat domain.AgentChat,
	journalDir, clientRequestID string,
	existing promptRequestRecord,
) (dto.PromptSubmissionDTO, bool, error) {
	accepted, err := u.promptRecordAccepted(ctx, chat, existing)
	if err != nil {
		return dto.PromptSubmissionDTO{}, true, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "recover prior delivery", err,
		)
	}
	if !accepted {
		return dto.PromptSubmissionDTO{}, true, ErrPromptOutcomeUnknown
	}
	if _, err := u.prompts.markAcceptedByRequest(journalDir, clientRequestID, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: submit prompt: persist recovered acceptance",
			"chat_id", chat.ID, "client_request_id", clientRequestID, "err", err)
	}
	return dto.PromptSubmissionDTO{}, true, ErrPromptAlreadyAccepted
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
	for _, t := range turns {
		if deliveredThisRequest(t, record) && promptTextHash(t.Text) == record.TextHash {
			return true, nil
		}
	}
	return false, nil
}

// deliveredThisRequest reports whether one recorded turn is evidence that THIS
// request's prompt reached a CLI.
//
// When the replacement runner id was journaled, only that runner's turn counts.
// When it was not — the process-commit crash gap — a turn from any runner EXCEPT
// the outgoing one is positive evidence; a legacy row with no runner attribution
// is deliberately insufficient, because "we cannot tell who delivered this" must
// never resolve to "we did".
func deliveredThisRequest(
	turn chatlog.Turn,
	record promptRequestRecord,
) bool {
	if turn.Role != "user" || turn.Provider != record.ProviderID || turn.At.Before(record.CreatedAt) {
		return false
	}
	if record.RunnerID != "" {
		return turn.RunnerID == record.RunnerID
	}
	return turn.RunnerID != "" && turn.RunnerID != record.OutgoingRunnerID
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

// promptDelivery is everything resolved before dispatch about HOW one message
// reaches the CLI: which channel carries it, the argv that carries it if that
// channel is a new process, whether that process continues the native
// conversation, and which conversation that is.
//
// The argv half is resolved EVEN WHEN the chosen channel is rewake, and that is
// deliberate. A rewake can fail to find a collector, and the only acceptable
// answer to that is to restart the CLI with the message in its argv — so the
// fallback has to be fully rendered and fully validated before the first
// destructive step, not assembled in the failure path where its own errors would
// arrive with a prompt already in flight.
type promptDelivery struct {
	rewake          bool
	promptSteps     []engineagents.InjectStep
	resumeSteps     []engineagents.InjectStep
	launchSessionID string
	resuming        bool
}

// resolvePromptDelivery renders the delivery for one message, and refuses it
// when the provider has no channel Crowbar can deliver through.
//
// It runs entirely before the durable dispatch intent is written, so every
// refusal here is a definitive client error rather than an outcome-unknown
// dispatch against a process that has already been torn down.
func (u *Usecase) resolvePromptDelivery(
	ctx context.Context,
	chatID string,
	live domain.AgentRunner,
	descriptor engineagents.Agent,
) (promptDelivery, error) {
	resuming, nativeSessionID, err := u.resumeTarget(ctx, chatID, live)
	if err != nil {
		return promptDelivery{}, err
	}
	// "Can this provider take a prompt at all" is asked BEFORE "must this one
	// replace the process": a terminal-only provider is unsupported for the same
	// reason whatever the chat has selected, and answering the second question
	// first would report the same refusal under a reason that is not the real one.
	promptSteps, err := descriptor.PromptSteps(resuming)
	if err != nil {
		if errors.Is(err, engineagents.ErrPromptSubmitUnsupported) {
			return promptDelivery{}, ErrPromptUnsupported
		}
		return promptDelivery{}, fmt.Errorf("agent: submit prompt: render prompt mapping: %w", err)
	}
	byRewake, err := u.promptDeliveryChannel(ctx, chatID, live, descriptor)
	if err != nil {
		return promptDelivery{}, err
	}
	out := promptDelivery{rewake: byRewake, promptSteps: promptSteps, resuming: resuming}
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

// resumeTarget answers whether the replacement process continues the native
// conversation this runner is in, and which conversation that is.
func (u *Usecase) resumeTarget(
	ctx context.Context,
	chatID string,
	live domain.AgentRunner,
) (bool, string, error) {
	// A TUI launched with an explicit native resume target is continuing that
	// conversation even when its session_start is still in flight, or when every
	// existing turn predates this runner's bind timestamp. The durable launch
	// identity is the authoritative answer for that case.
	if live.LaunchSessionID != "" &&
		(live.CurrentSession == "" || live.CurrentSession == live.LaunchSessionID) {
		return true, live.LaunchSessionID, nil
	}
	if live.CurrentSession == "" {
		return false, live.CurrentSession, nil
	}
	// A native TUI /resume is learned from the same append-only session history as
	// Crowbar's own ResumeChat. Its existing turns legitimately predate this
	// binding, so the persisted known-session fact wins over the timestamp
	// heuristic without any provider-specific hook vocabulary.
	if live.CurrentSessionResumable || live.CurrentSessionSince.IsZero() {
		return live.CurrentSessionResumable, live.CurrentSession, nil
	}
	resuming, err := u.activity.HasTurnAtOrAfter(ctx, chatID, live.ProviderID, live.CurrentSessionSince)
	if err != nil {
		return false, "", fmt.Errorf("agent: submit prompt: inspect current conversation: %w", err)
	}
	return resuming, live.CurrentSession, nil
}

// promptDeliveryChannel picks the channel this one message travels on, and
// refuses a message Crowbar has no channel for at all.
//
// TWO INDEPENDENT AUTHORITIES can insist on a REPLACEMENT PROCESS, and they are
// asked in this order because the reasons are not interchangeable:
//
//   - the provider's declared delivery strategy. restart_tui — the portable floor,
//     and what codex declares — respawns the CLI for every message, so the answer
//     is already "restart" before anything else is asked. Asking it first is also
//     what keeps that provider's path byte-identical to the one that existed
//     before any other channel did: not one extra read is performed for it.
//   - a PENDING SELECTION SWITCH: the chat's model or effort has moved since this
//     process was launched, and the model/effort block declares that a change
//     takes effect on the next process. This one holds ON ITS OWN and it OUTRANKS
//     the declared strategy, because a running CLI's model cannot be changed in
//     place — delivering to the live session would silently run the message under
//     the model the user just changed away from.
//
// Only with both of those satisfied does a rewake-declaring provider get its
// optimisation. And when neither authority says restart and the strategy is one
// this daemon does not implement, the message is refused rather than delivered by
// a mechanism its descriptor did not ask for — unreachable for both shipped
// descriptors, reachable for an on-disk override, and precisely the case that
// must not be answered by guessing.
func (u *Usecase) promptDeliveryChannel(
	ctx context.Context,
	chatID string,
	live domain.AgentRunner,
	descriptor engineagents.Agent,
) (byRewake bool, err error) {
	if descriptor.Capabilities().Delivery == engineagents.DeliveryRestartTUI {
		return false, nil
	}
	desired, err := u.chatSelection(ctx, chatID, false)
	if err != nil {
		return false, err
	}
	launched := engineagents.Selection{Model: live.LaunchModel, Effort: live.LaunchEffort}
	if descriptor.SelectionRestart(launched, desired) {
		return false, nil
	}
	if descriptor.Capabilities().Delivery == engineagents.DeliveryRewakeHook {
		return true, nil
	}
	return false, ErrPromptUnsupported
}

// deliverToLiveSession hands one message to a collector blocked on the live CLI,
// and reports whether this submission is finished.
//
// done=false is the ONLY answer a caller may act on destructively, and it means
// exactly one thing: no collector took these bytes, so the message is still
// entirely undelivered and restarting the CLI with it in argv cannot duplicate
// anything. Every other outcome is done=true — delivered, or delivered-and-unsure
// — because past the handoff there is nothing to take back, and a prompt whose
// outcome is unknown must never be sent a second time on a guess.
func (u *Usecase) deliverToLiveSession(
	ctx context.Context,
	journalDir, clientRequestID, textHash, chatID string,
	live domain.AgentRunner,
	text string,
) (dto.PromptSubmissionDTO, bool, error) {
	// Checked BEFORE the handoff, not after: without a terminal identity there is
	// no result to return, and finding that out with the message already collected
	// would turn a fallback into an unknown outcome for no reason.
	if live.TerminalSession == "" {
		return dto.PromptSubmissionDTO{}, false, nil
	}
	handed, acked := u.rewake.deliver(
		live.ID, chatID, text, rewakeHandoffBudget, rewakeAckBudget,
	)
	if !handed {
		return dto.PromptSubmissionDTO{}, false, nil
	}
	if !acked {
		// A collector took the message and then went quiet. The bytes may be with
		// its CLI already, so this is the uncertain class the journal exists for and
		// never the retry class.
		return dto.PromptSubmissionDTO{}, true, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "rewake collector never acknowledged the handoff", nil,
		)
	}
	result := dto.PromptSubmissionDTO{RunnerID: live.ID, TerminalSessionID: live.TerminalSession}
	committed, err := u.prompts.markSpawned(journalDir, clientRequestID, textHash, result, time.Now())
	if err != nil {
		return dto.PromptSubmissionDTO{}, true, u.markPromptOutcomeUncertain(
			ctx, journalDir, clientRequestID, "persist rewake delivery", err,
		)
	}
	if committed.State == promptStateUncertain {
		return dto.PromptSubmissionDTO{}, true, ErrPromptOutcomeUnknown
	}
	return result, true, nil
}
