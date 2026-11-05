package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const maxReactPromptBytes = 64 * 1024

func (rs *Runners) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID string,
) (domain.AgentPromptSubmission, error) {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: %w", err)
	}
	defer h.release()
	return rs.submitPromptLocked(ctx, h, chatID, text, clientRequestID, nil)
}

// submitPromptLocked is SubmitPrompt's body under h. admit, if set, runs once the
// request is known to be new (a resend answers from the ledger and changes
// nothing); handled says admit delivered the prompt itself.
func (rs *Runners) submitPromptLocked(
	ctx context.Context,
	h *hold,
	chatID, text, clientRequestID string,
	admit func(chat domain.Chat, requestID string) (domain.AgentPromptSubmission, bool, error),
) (domain.AgentPromptSubmission, error) {
	clientRequestID, err := normalisePromptRequest(text, clientRequestID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: chat: %w", err)
	}
	// Never decided against a runner whose end is under way: that end's owner
	// may be about to put its own prompt on the chat.
	if _, _, err := rs.settledRunner(h); err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: %w", err)
	}
	if result, done, err := rs.replayPriorPrompt(ctx, chat.ID, clientRequestID, text); done {
		return result, err
	}
	if admit != nil {
		if submission, handled, err := admit(chat, clientRequestID); handled || err != nil {
			return submission, err
		}
	}

	live, descriptor, worktree, err := rs.promptTarget(ctx, chat)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	if submission, handled, steerErr := rs.submitPromptSteered(
		ctx, chat, clientRequestID, live, descriptor, text,
	); handled {
		return submission, steerErr
	}

	if err := rs.requirePromptIdle(ctx, chatID, live.ID); err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	if submission, handled, pushErr := rs.submitPromptOverAPI(
		ctx, chat, clientRequestID, live, descriptor, worktree, text,
	); handled {
		return submission, pushErr
	}

	delivery, err := rs.resolvePromptDelivery(ctx, chat.ID, live.ProviderID, live, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	return rs.launchCarrying(ctx, h, chat, clientRequestID, text, delivery)
}

// spawnFailure is what a failed replacement spawn tells the client: a CLI that
// is not installed, or was refused its session, provably never took the prompt;
// any other failure may follow a fork.
func spawnFailure(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, engineterminal.ErrCommandNotFound):
		return fmt.Errorf("agent: submit prompt: replacement spawn: %w", err)
	case errors.Is(err, ErrSpawnSessionRefused):
		return ErrPromptSessionLost
	}
	return uncertainPromptOutcome(ctx, "replacement spawn", err)
}

// pushFailure is what a failed api push tells the client: a session the
// provider no longer has cannot have taken the prompt.
func pushFailure(ctx context.Context, err error) error {
	if errors.Is(err, engineagents.ErrAPISessionLost) {
		return ErrPromptSessionLost
	}
	return uncertainPromptOutcome(ctx, "api push", err)
}

func normalisePromptRequest(
	text, clientRequestID string,
) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("agent: submit prompt: text is required: %w", problem.ErrInvalidArgument)
	}
	if len([]byte(text)) > maxReactPromptBytes {
		return "", fmt.Errorf("agent: submit prompt: text exceeds %d bytes: %w",
			maxReactPromptBytes, problem.ErrInvalidArgument)
	}

	if strings.IndexByte(text, 0) >= 0 {
		return "", fmt.Errorf("agent: submit prompt: text contains an unsupported NUL byte: %w",
			problem.ErrInvalidArgument)
	}
	requestUUID, err := uuid.Parse(clientRequestID)
	if err != nil {
		return "", fmt.Errorf("agent: submit prompt: clientRequestId must be a UUID: %w",
			problem.ErrInvalidArgument)
	}
	return requestUUID.String(), nil
}

// replayPriorPrompt answers a request the ledger already holds, and refuses a
// new one while another prompt is pending. done=false: the request is new.
func (rs *Runners) replayPriorPrompt(
	ctx context.Context,
	chatID, requestID, text string,
) (domain.AgentPromptSubmission, bool, error) {
	st, err := rs.settleOrphanedPrompts(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: %w", err)
	}
	prior, replay, err := priorPrompt(st, requestID, text)
	if err != nil || !replay {
		return domain.AgentPromptSubmission{}, err != nil, err
	}
	carrier, ok := rs.live.byRunner(prior.RunnerID)
	if !ok {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: resend: carrier: %w", ErrNoLiveRunner)
	}
	return domain.AgentPromptSubmission{RunnerID: prior.RunnerID, TerminalSessionID: carrier.TerminalSession}, true, nil
}

// settleOrphanedPrompts settles the chat's pending prompts whose carrier is not
// its live runner, then returns the State. A departure that never settled its
// prompt would otherwise hold the chat busy for good.
func (rs *Runners) settleOrphanedPrompts(ctx context.Context, chatID string) (ledger.State, error) {
	live, _ := rs.live.byChat(chatID)
	_, st, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.SettleOrphaned(st, func(runnerID string) bool { return runnerID == live.ID })
	})
	if err != nil {
		return ledger.State{}, fmt.Errorf("pending prompts: ledger: %w", err)
	}
	return st, nil
}

// priorPrompt is the ledger's verdict on requestID. The same text is a resend
// (replay); other text, or a prompt already settled, is a conflict; a new
// request waits behind any prompt still pending.
func priorPrompt(st ledger.State, requestID, text string) (ledger.Prompt, bool, error) {
	p, ok := st.Receipt(requestID)
	switch {
	case ok && (p.Text != text || p.Status == ledger.PromptStatusSettled):
		return p, false, ErrPromptRequestIDConflict
	case ok:
		return p, true, nil
	case len(st.PendingPrompts()) > 0:
		return p, false, ErrPromptBusy
	}
	return p, false, nil
}

// recordPrompt appends requestID's user_message before any CLI is touched: from this
// commit the prompt is owed an outcome. dispatch is what the CLI is handed if not
// text; carrier is the delivering runner; admit, when set, may refuse the record.
func (rs *Runners) recordPrompt(
	ctx context.Context, chatID, requestID, text, dispatch, delivery, carrier string, admit func(ledger.State) error,
) error {
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		if admit != nil {
			if err := admit(st); err != nil {
				return nil, err
			}
		}
		if _, replay, err := priorPrompt(st, requestID, text); err != nil || replay {
			if err == nil {
				err = ErrPromptRequestIDConflict
			}
			return nil, err
		}
		if dispatch == text {
			dispatch = ""
		}
		row, err := ledger.NewRow(ledger.KindUserMessage, uuid.NewString(), "", ledger.UserMessage{
			RequestID: requestID, Text: text, Origin: ledger.OriginCrowbar, Delivery: delivery, RunnerID: carrier,
			Dispatch: dispatch,
		})
		return []ledger.Row{row}, err
	})
	if err != nil {
		return fmt.Errorf("agent: submit prompt: record: %w", err)
	}
	return nil
}

// failPrompt settles requestID as undelivered, as lost when cause says its
// session was, and returns cause.
func (rs *Runners) failPrompt(ctx context.Context, chatID, requestID string, cause error) error {
	reason := ledger.ReasonDeliveryFailed
	if errors.Is(cause, ErrPromptSessionLost) {
		reason = ledger.ReasonSessionLost
	}
	rs.settlePrompts(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Settle(st, func(p ledger.Prompt) bool { return p.RequestID == requestID }, false, reason)
	})
	return cause
}

// settlePrompts commits decide's settles on chatID. It outlives ctx: a prompt
// left pending would hold the chat busy.
func (rs *Runners) settlePrompts(ctx context.Context, chatID string, decide func(ledger.State) ([]ledger.Row, error)) {
	ctx = context.WithoutCancel(ctx)
	if _, _, err := rs.ledger.Do(ctx, chatID, decide); err != nil {
		slog.ErrorContext(ctx, "agent: settle pending prompt", "chat_id", chatID, "err", err)
	}
}

// submitPromptOverAPI delivers text over live's api connection with no restart, unless
// the selection changed in a restart_tui field (the connection would keep the OLD
// model). handled=false — no connection, or such a change — falls back to a restart.
func (rs *Runners) submitPromptOverAPI(
	ctx context.Context,
	chat domain.Chat,
	clientRequestID string,
	live engineagents.Runner,
	descriptor engineagents.Agent,
	worktree, text string,
) (domain.AgentPromptSubmission, bool, error) {
	if _, ok := rs.apiConns.get(live.ID); !ok {
		return domain.AgentPromptSubmission{}, false, nil
	}
	restart, err := rs.selectionRequiresRestart(ctx, chat.ID, live, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf(
			"agent: submit prompt: check selection restart: %w", err,
		)
	}
	if restart {
		return domain.AgentPromptSubmission{}, false, nil
	}

	// A COPY of text for dispatch — the durable ledger text is never mutated.
	dispatchText, err := rs.rewritePromptTextForDispatch(ctx, chat.WorkspaceID, chat.ID, text)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, fmt.Errorf("agent: submit prompt: materialize attachments: %w", err)
	}
	if err := rs.recordPrompt(ctx, chat.ID, clientRequestID, text, "", ledger.DeliveryAPI, live.ID, nil); err != nil {
		return domain.AgentPromptSubmission{}, true, err
	}
	session, err := rs.boundSession(ctx, chat.ID, live.ProviderID)
	if err != nil {
		return domain.AgentPromptSubmission{}, true, rs.failPrompt(ctx, chat.ID, clientRequestID, err)
	}
	_, _, pushErr := rs.pushPromptOverAPI(ctx, live.ID, session, worktree, dispatchText)
	if pushErr != nil {
		rs.loseAPISession(ctx, chat.ID, live, session, pushErr)
		return domain.AgentPromptSubmission{}, true, rs.failPrompt(
			ctx, chat.ID, clientRequestID, pushFailure(ctx, pushErr),
		)
	}
	submission, err := rs.commitPromptSpawn(ctx, chat.ID, clientRequestID, live.ID)
	return submission, true, err
}

// displaceForPrompt quits the chat's idle runner outside the chat's lock, then
// refuses the replacement unless requestID is still the chat's to deliver and
// nothing took the chat meanwhile.
func (rs *Runners) displaceForPrompt(
	ctx context.Context,
	h *hold,
	chatID, requestID, liveRunnerID string,
) error {
	if err := rs.endIdleForPrompt(ctx, chatID, liveRunnerID); err != nil {
		return err
	}
	if err := h.outside(func(context.Context) error { return rs.quitOutgoingCLI(ctx, chatID) }); err != nil {
		return err
	}
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: submit prompt: %w", err)
	}
	p, _ := st.Receipt(requestID)
	if _, taken := rs.live.byChat(chatID); taken || p.Status != ledger.PromptStatusPending {
		return ErrPromptBusy
	}
	return nil
}

// endIdleForPrompt refuses a busy chat and marks its idle runner ending, in
// its turn order: no turn opens on it before the quit.
func (rs *Runners) endIdleForPrompt(ctx context.Context, chatID, liveRunnerID string) (err error) {
	rs.live.inTurnOrder(chatID, func() {
		if err = rs.requirePromptIdle(ctx, chatID, liveRunnerID); err == nil {
			rs.markOutgoingEnding(chatID)
		}
	})
	return err
}

// commitPromptSpawn answers with the runner the prompt went to. What must exist
// is a DELIVERY TARGET, not a PTY: an api-driven runner forks none
// (apirunner.go), and its connection is that target.
func (rs *Runners) commitPromptSpawn(
	ctx context.Context,
	chatID, clientRequestID, runnerID string,
) (domain.AgentPromptSubmission, error) {
	spawned, ok := rs.live.byRunner(runnerID)
	if !ok {
		// The carrier existed and may have acknowledged it; its exit settled it.
		return domain.AgentPromptSubmission{}, uncertainPromptOutcome(ctx, "read replacement runner", ErrNoLiveRunner)
	}
	result := domain.AgentPromptSubmission{
		RunnerID:          runnerID,
		TerminalSessionID: spawned.TerminalSession,
	}
	if result.TerminalSessionID == "" && !rs.HasLiveAPIConnection(runnerID) {
		return domain.AgentPromptSubmission{}, rs.failPrompt(ctx, chatID, clientRequestID,
			uncertainPromptOutcome(ctx, "replacement delivery identity is missing", nil))
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

func (rs *Runners) requirePromptIdle(ctx context.Context, chatID, runnerID string) error {
	current, ok := rs.live.byChat(chatID)
	if !ok {
		return ErrPromptSessionUnavailable
	}
	if current.ID != runnerID {
		return ErrPromptBusy
	}
	return rs.requireNotBusy(ctx, chatID)
}

// requireChatAcceptingPrompt is the busy verdict a send would otherwise reach
// only after its staged selection was committed.
func (rs *Runners) requireChatAcceptingPrompt(ctx context.Context, chatID string) error {
	if err := rs.requireNotBusy(ctx, chatID); err != nil {
		return err
	}
	return rs.requireNoPendingPrompt(ctx, chatID)
}

// requireNotBusy refuses while the chat's ledger folds it working.
func (rs *Runners) requireNotBusy(ctx context.Context, chatID string) error {
	busy, err := rs.ChatBusy(ctx, chatID)
	if err != nil {
		return err
	}
	if busy {
		return ErrPromptBusy
	}
	return nil
}

// ChatBusy reports whether chatID's ledger folds it working.
func (rs *Runners) ChatBusy(ctx context.Context, chatID string) (bool, error) {
	busy, _, err := rs.ChatWork(ctx, chatID)
	return busy, err
}

// ChatWork reports whether chatID's ledger folds it working, and whether a
// prompt still awaits its provider.
func (rs *Runners) ChatWork(ctx context.Context, chatID string) (busy, awaiting bool, err error) {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return false, false, fmt.Errorf("agent: chat work: %w", err)
	}
	return st.Busy(), len(st.PendingPrompts()) > 0, nil
}

// requireNoPendingPrompt refuses while a prompt awaits the provider, once those
// whose carrier has left the chat are settled.
func (rs *Runners) requireNoPendingPrompt(ctx context.Context, chatID string) error {
	st, err := rs.settleOrphanedPrompts(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: prompt guard: %w", err)
	}
	if len(st.PendingPrompts()) > 0 {
		return ErrPromptBusy
	}
	return nil
}

// reconcilePendingPromptsOnBoot settles every prompt a restart left pending: no
// carrier from before it can acknowledge one now.
func (rs *Runners) reconcilePendingPromptsOnBoot(ctx context.Context) error {
	chats, err := rs.ledger.OpenChats(ctx)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile pending prompts: %w", err)
	}
	every := func(ledger.Prompt) bool { return true }
	var errs []error
	for _, chatID := range chats {
		if _, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
			return ledger.Settle(st, every, false, ledger.ReasonDaemonRestart)
		}); err != nil {
			errs = append(errs, fmt.Errorf("agent: boot reconcile pending prompts: %s: %w", chatID, err))
		}
	}
	return errors.Join(errs...)
}
