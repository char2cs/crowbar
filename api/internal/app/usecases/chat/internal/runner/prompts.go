package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const maxReactPromptBytes = 64 * 1024

func (rs *Runners) SubmitPrompt(
	ctx context.Context,
	chatID, text, clientRequestID string,
) (domain.AgentPromptSubmission, error) {
	_, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	defer release()
	return rs.submitPromptLocked(ctx, chatID, text, clientRequestID, nil)
}

// submitPromptLocked is SubmitPrompt's body, split out so
// SubmitPromptWithSwitch (promptswitch.go) can run it as the TAIL of its own
// single gate hold — a staged provider switch and selection commit, then this,
// never releasing the gate in between. See that file's own doc for why.
//
// admit (nil for none) runs once the request is known to be new — a resend of
// a recorded prompt answers from the ledger and must never spawn or mutate
// anything.
func (rs *Runners) submitPromptLocked(
	ctx context.Context,
	chatID, text, clientRequestID string,
	admit func() error,
) (domain.AgentPromptSubmission, error) {
	clientRequestID, err := normalisePromptRequest(text, clientRequestID)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return domain.AgentPromptSubmission{}, fmt.Errorf("agent: submit prompt: chat: %w", err)
	}
	if result, done, err := rs.replayPriorPrompt(ctx, chat.ID, clientRequestID, text); done {
		return result, err
	}
	if admit != nil {
		if err := admit(); err != nil {
			return domain.AgentPromptSubmission{}, err
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

	delivery, err := rs.resolvePromptDelivery(ctx, chat.ID, live, descriptor)
	if err != nil {
		return domain.AgentPromptSubmission{}, err
	}

	replacementRunnerID := uuid.NewString()
	if err := rs.recordPrompt(
		ctx, chat.ID, clientRequestID, text, ledger.DeliveryRestart, replacementRunnerID, nil,
	); err != nil {
		return domain.AgentPromptSubmission{}, err
	}
	if err := rs.displaceForPrompt(ctx, chatID, live.ID); err != nil {
		return domain.AgentPromptSubmission{}, rs.failPrompt(ctx, chat.ID, clientRequestID, err)
	}

	runnerID, err := rs.spawnRunner(ctx, chatID, chat.WorkspaceID, live.ProviderID, spawnInput{
		runnerID: replacementRunnerID, launch: &delivery.launch, finalSteps: delivery.promptSteps,
		conversation: delivery.conversation, resuming: delivery.contextResuming, promptMessage: text,
	}, false)
	if err != nil {
		return domain.AgentPromptSubmission{}, rs.failPrompt(ctx, chat.ID, clientRequestID, spawnFailure(ctx, err))
	}
	return rs.commitPromptSpawn(ctx, chat.ID, clientRequestID, runnerID)
}

// spawnFailure is what a failed replacement spawn tells the client: only a CLI
// that is not installed provably never ran, any other failure may follow a fork.
func spawnFailure(ctx context.Context, err error) error {
	if errors.Is(err, engineterminal.ErrCommandNotFound) {
		return fmt.Errorf("agent: submit prompt: replacement spawn: %w", err)
	}
	return uncertainPromptOutcome(ctx, "replacement spawn", err)
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

// recordPrompt appends requestID's user_message before any CLI is touched: the
// prompt exists, and is owed an outcome, from this commit on. carrier is the
// runner that will deliver it; admit, when set, refuses the record.
func (rs *Runners) recordPrompt(
	ctx context.Context, chatID, requestID, text, delivery, carrier string, admit func(ledger.State) error,
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
		row, err := ledger.NewRow(ledger.KindUserMessage, uuid.NewString(), "", ledger.UserMessage{
			RequestID: requestID, Text: text, Origin: ledger.OriginCrowbar, Delivery: delivery, RunnerID: carrier,
		})
		return []ledger.Row{row}, err
	})
	if err != nil {
		return fmt.Errorf("agent: submit prompt: record: %w", err)
	}
	return nil
}

// failPrompt settles requestID as undelivered and returns cause.
func (rs *Runners) failPrompt(ctx context.Context, chatID, requestID string, cause error) error {
	rs.settleUndelivered(ctx, chatID, requestID)
	return cause
}

func (rs *Runners) settleUndelivered(ctx context.Context, chatID, requestID string) {
	rs.settlePrompts(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Settle(st, func(p ledger.Prompt) bool { return p.RequestID == requestID },
			false, ledger.ReasonDeliveryFailed)
	})
}

// settlePrompts commits decide's settles on chatID. It outlives ctx: a prompt
// left pending would hold the chat busy.
func (rs *Runners) settlePrompts(ctx context.Context, chatID string, decide func(ledger.State) ([]ledger.Row, error)) {
	ctx = context.WithoutCancel(ctx)
	if _, _, err := rs.ledger.Do(ctx, chatID, decide); err != nil {
		slog.ErrorContext(ctx, "agent: settle pending prompt", "chat_id", chatID, "err", err)
	}
}

// submitPromptOverAPI delivers text over live's connection when it has one —
// the mixed-transport case, where MOST messages need no restart at all: the
// same connection applyAPITransport opened at spawn carries every message
// the conversation sends, first or hundredth, AS LONG AS the chat's desired
// model/effort/permission level still match what this connection's process
// was launched with. A changed selection that the descriptor itself declares
// restart_tui for (selectionRequiresRestart, promptdelivery.go) is refused
// here instead: pushing it over the stale connection would silently keep
// running the OLD model/effort forever, since nothing on this fast path ever
// re-examines the launch argv a running process was started with — confirmed
// live, this exact gap is why a codex model/effort switch never reached the
// CLI. handled=false means either no live api connection (every hooks-only
// provider, and a mixed-transport one whose serve process never came up) or
// a selection change this connection cannot carry; either way the caller
// falls back to the restart_tui path, which tears the connection down and
// respawns with the new selection.
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
	if err := rs.recordPrompt(ctx, chat.ID, clientRequestID, text, ledger.DeliveryAPI, live.ID, nil); err != nil {
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
			ctx, chat.ID, clientRequestID, uncertainPromptOutcome(ctx, "api push", pushErr),
		)
	}
	submission, err := rs.commitPromptSpawn(ctx, chat.ID, clientRequestID, live.ID)
	return submission, true, err
}

func (rs *Runners) promptTarget(
	ctx context.Context,
	chat domain.Chat,
) (engineagents.Runner, engineagents.Agent, string, error) {
	live, ok := rs.live.byChat(chat.ID)
	if !ok {
		return engineagents.Runner{}, nil, "", ErrPromptSessionUnavailable
	}
	cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chat.ID, chat.WorkspaceID)
	if err != nil {
		return engineagents.Runner{}, nil, "", err
	}
	crowbarHome, _, _, worktree, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
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
	chatID, liveRunnerID string,
) error {
	if err := rs.endIdleForPrompt(ctx, chatID, liveRunnerID); err != nil {
		return err
	}
	return rs.quitOutgoingCLI(ctx, chatID)
}

// endIdleForPrompt refuses a busy chat and marks its idle runner ending, under
// the chat's turn-start gate: no turn opens on it before the quit.
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

// PendingPrompt is the chat's oldest prompt still waiting on the provider, so a
// client that lost its own copy can show the text back.
func (rs *Runners) PendingPrompt(ctx context.Context, chatID string) (domain.PendingPrompt, bool, error) {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return domain.PendingPrompt{}, false, fmt.Errorf("agent: pending prompt: %w", err)
	}
	pending := st.PendingPrompts()
	if len(pending) == 0 {
		return domain.PendingPrompt{}, false, nil
	}
	return domain.PendingPrompt{
		Text: pending[0].Text, State: string(ledger.PromptStatusPending), RequestID: pending[0].RequestID,
	}, true, nil
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
