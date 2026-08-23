package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

func (u *runnerUsecase) displace(
	ctx context.Context,
	runner domain.AgentRunner,
) error {
	vacated := runner.CurrentChatID

	if _, err := u.runners.Displace(ctx, runner.ID); err != nil {
		if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, agentrunner.ErrNotFound) {
			slog.WarnContext(ctx, "agent: displace runner: it had already exited (benign)",
				"runner_id", runner.ID, "chat_id", vacated)
			u.turns.complete(runner.ID)
			return nil
		}
		return fmt.Errorf("agent: displace runner: %w", err)
	}
	// Once displacement commits this runner can no longer deliver a hook into the
	// vacated chat. Resolve any pending React dispatch from the hook-derived ledger,
	// or mark it safely retryable when no matching user turn exists.
	u.reconcilePromptRunnerDeparture(ctx, runner, vacated)

	// A displaced runner is on NO chat, so handleTurn drops every hook it has left —
	// including the turn_stop that would have ended a turn it is still mid-way through.
	// Nothing will ever close that turn where it stood, so anybody waiting on it is
	// released here or waits for their context to die (turnWaits.complete).
	u.turns.complete(runner.ID)
	u.closeAbandonedTurn(ctx, vacated)
	return nil
}

func (u *runnerUsecase) closeAbandonedTurn(
	ctx context.Context,
	chatID string,
) {
	if chatID == "" {
		return
	}
	if _, err := u.runners.LiveRunnerForChat(ctx, chatID); err == nil {
		return // someone else is on this chat now: its turn is not ours to close
	}
	// AbandonTurn, not StopTurn: the CLI is GONE, so it will never restate the level of
	// async work it last reported outstanding, and a plain StopTurn would leave that
	// number standing — Working is folded from the turn OR that level, so the chat would
	// spin forever on work nothing is doing. That is the same wedge this function exists
	// to prevent, one field over. Nothing a dead CLI announced survives it.
	//
	// This is the ONLY thing standing between a killed CLI and a permanently-spinning
	// chat: measured against claude 2.1.212, a SIGKILL mid-background-work sends no
	// SessionEnd and no final Stop — the last word is a turn_stop reporting work still
	// running, and in an event-sourced aggregate that word outlives the restart.
	// The conversation record is closed FIRST and unconditionally. A dead CLI's
	// turn is open there too, holding whatever tool calls it had in flight — and
	// those render as running for as long as the record says so, which is forever.
	// The next turn's activity would attach to that stale open turn as well.
	if err := u.activity.Abandon(ctx, chatID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close abandoned turn: abandon conversation record",
			"chat_id", chatID, "err", err)
	}

	abandoned, err := u.chats.AbandonTurn(ctx, chatID, time.Now())
	if err == nil {
		u.work.set(chatID, abandoned.Working)
		return
	}
	if errors.Is(err, asynxModels.ErrValidation) {
		// The command's authoritative fold says there is nothing to abandon.
		u.work.set(chatID, false)
		return
	}
	slog.WarnContext(ctx, "agent: close abandoned turn: abandon turn", "chat_id", chatID, "err", err)
}

func (u *runnerUsecase) ReconcileRunnersOnBoot(
	ctx context.Context,
) error {
	runners, err := u.runners.AllLive(ctx)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile: list runners: %w", err)
	}
	for _, r := range runners {
		// SessionLive is the seam that asks "is this PROCESS alive", NOT the engine's
		// SessionExists, which is also true for a PTY-less suspended placeholder — a session
		// whose process is already dead and whose only remaining substance is scrollback on
		// disk. Restoring those placeholders is the boot step immediately before this one, so
		// asking the registry "do you know this id?" here would answer yes for every single
		// dead agent CLI and reconcile NOTHING. That is the exact mistake that let a
		// restart-orphaned chat go on advertising a live agent.
		if u.term.SessionLive(ctx, r.TerminalSession) {
			continue
		}
		// Read the placement BEFORE the Exit: it is the chat whose turn this CLI abandoned.
		abandoned := r.CurrentChatID

		// The tmp reap below is intentionally gated on a SUCCESSFUL Exit, even though the
		// PTY is already known dead at this point and the dir could be reaped regardless.
		// Coupling them keeps the row and its tmp dir moving together: if Exit fails, the
		// row is still recorded live, so this same runner reappears in AllLive on the next
		// boot and both the Exit and the reap are retried then. Reaping now while Exit's
		// error leaves the row live would desync the two — a "live" row with its config
		// already gone — for no gain, since the dir holds no credential and an extra boot
		// of staleness is the same benign wait the surrounding best-effort loop already
		// accepts everywhere else.
		if _, err := u.runners.Exit(ctx, r.ID, time.Now()); err != nil {
			slog.ErrorContext(ctx, "agent: boot reconcile: exit dead runner (best-effort, continuing)",
				"runner_id", r.ID, "terminal_session_id", r.TerminalSession, "err", err)
			continue
		}
		u.reconcilePromptRunnerDeparture(ctx, r, r.CurrentChatID)
		u.reapCrashOrphanRunnerTmp(ctx, r)

		// Close the turn it died in the middle of. Turn state has never been durable truth
		// (domain.AgentChat.Working is documented as reconciled, not authoritative — a CLI
		// that dies mid-turn never sends the turn_stop hook that would close it), so
		// repairing it asserts nothing about any process. Without this, a chat that was
		// working when the daemon died comes back spinning and spins forever, and the
		// workspace's whole overlay spins with it.
		//
		// The Exit above is SendWait, so the "is anyone still on this chat" read inside can
		// no longer see the runner we have just reaped.
		u.closeAbandonedTurn(ctx, abandoned)
	}
	if err := u.reconcilePromptJournalsOnBoot(ctx); err != nil {
		return err
	}
	return nil
}

func (u *runnerUsecase) reconcileRunnerExit(ctx context.Context, runnerID string) {
	// The echo guard is per-spawn and means nothing once the process is gone.
	u.agents.ForgetRunner(runnerID)
	// Nor does a prompt it was blocked on. Its hooks may still be ALIVE — they are
	// spawned detached, so killing a CLI orphans them — so every relay this runner
	// owned is woken with no verdict, and every question it was asking is closed.
	// A pending prompt over a process that no longer exists is a banner nothing
	// else will ever clear.
	u.answers.releaseAnswerWaiters(ctx, runnerID)
	// Nor does an in-flight turn: a CLI that died mid-answer will never send the turn_stop
	// that closes it, so a provider switch parked on that turn is released by the DEATH
	// instead — the same real signal that ends the runner, and the reason the wait needs no
	// timeout to be safe against a CLI that falls over.
	u.turns.complete(runnerID)

	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		if !errors.Is(err, agentrunner.ErrNotFound) {
			slog.WarnContext(ctx, "agent: reconcile runner exit: get runner", "runner_id", runnerID, "err", err)
		}
		// Already exited (a double exit is not an error — the row is simply gone).
		return
	}
	u.reconcilePromptRunnerDeparture(ctx, runner, runner.CurrentChatID)
	if _, err := u.runners.Exit(ctx, runnerID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: reconcile runner exit: exit runner", "runner_id", runnerID, "err", err)
		return
	}

	// Close a turn it left open — unless it had already been DISPLACED, in which case its
	// chat (if it still had a turn to close) was dealt with at displacement time and
	// CurrentChatID is now empty, meaning nowhere.
	u.closeAbandonedTurn(ctx, runner.CurrentChatID)
}

func (u *runnerUsecase) retireChatRunners(
	ctx context.Context,
	chatID string,
) {
	placed, err := u.runners.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: look up runners on chat (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return
	}
	for _, r := range placed {
		u.retire(ctx, r)
	}
}

func (u *runnerUsecase) retire(
	ctx context.Context,
	runner domain.AgentRunner,
) {
	if err := u.displace(ctx, runner); err != nil {
		// Best-effort: the runner is still on its chat, so its own exit will close any turn
		// it leaves open. We still kill it.
		slog.ErrorContext(ctx, "agent: retire runner: displace (best-effort, continuing)",
			"runner_id", runner.ID, "chat_id", runner.CurrentChatID, "err", err)
	}
	if err := u.term.TerminateGraceful(ctx, runner.TerminalSession); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: retire runner: terminate (best-effort, continuing)",
			"runner_id", runner.ID, "terminal_session_id", runner.TerminalSession, "err", err)
	}
}

func (u *runnerUsecase) reapCrashOrphanRunnerTmp(
	ctx context.Context,
	runner domain.AgentRunner,
) {
	chatsDir, err := u.ws.AgentChatsDir(ctx, runner.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: chats dir (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", runner.WorkspaceID, "err", err)
		return
	}
	home, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: home (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", runner.WorkspaceID, "err", err)
		return
	}
	RemoveUnderHome(ctx, home, worktreepath.RunnerDir(chatsDir, runner.ID, runner.ProviderID))
}

func (u *runnerUsecase) retireOthersOn(
	ctx context.Context,
	chatID string,
	keepID string,
) {
	placed, err := u.runners.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: look up runners placed on chat (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return
	}
	u.retireOlderThan(ctx, placed, keepID, "chat", chatID)
}

func (u *runnerUsecase) evictHolderOf(
	ctx context.Context,
	runner domain.AgentRunner,
	sessionID string,
) {
	holders, err := u.runners.LiveRunnersForSession(ctx, runner.WorkspaceID, sessionID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: look up runners holding this conversation (best-effort, continuing)",
			"session_id", sessionID, "err", err)
		return
	}
	u.retireOlderThan(ctx, holders, runner.ID, "conversation", sessionID)
}

func (u *runnerUsecase) retireOlderThan(
	ctx context.Context,
	present []domain.AgentRunner,
	keepID string,
	whereKind string,
	whereID string,
) {
	me := -1
	for i, r := range present {
		if r.ID == keepID {
			me = i
			break
		}
	}
	if me < 0 {
		return
	}
	for _, older := range present[me+1:] {
		slog.WarnContext(ctx, "agent: another CLI was already here; retiring it",
			whereKind, whereID, "keeping", keepID, "retiring", older.ID)
		u.retire(ctx, older)
	}
}

func (u *runnerUsecase) requirePlacement(
	ctx context.Context,
	runner domain.AgentRunner,
	chatID string,
) (bool, error) {
	if chatID == "" {
		// Already displaced (we are killing it, and it has not fallen over yet).
		u.retire(ctx, runner)
		return false, nil
	}
	_, err := u.chats.GetChat(ctx, chatID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, agentchat.ErrNotFound) {
		return false, fmt.Errorf("agent: ingest hook: chat: %w", err)
	}
	slog.WarnContext(ctx, "agent: ingest hook: the runner's chat no longer exists; dropping the announcement and retiring the CLI",
		"runner_id", runner.ID, "chat_id", chatID)
	u.retire(ctx, runner)
	return false, nil
}

func (u *runnerUsecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

func (u *runnerUsecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]domain.ChatConversation, error) {
	return u.runners.ConversationsForChat(ctx, chatID)
}

func (u *runnerUsecase) ResumeChat(
	ctx context.Context,
	chatID string,
) (string, error) {
	defer u.spawns.lock(chatID)()

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if err == nil {
		return live.ID, nil
	}
	if !errors.Is(err, agentrunner.ErrNotFound) {
		return "", fmt.Errorf("agent: resume chat: live runner: %w", err)
	}
	last, err := u.runners.LastConversation(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resume chat: no conversation to resume: %w", err)
	}
	// The gate is already held: call the inner body, never SwitchProvider itself.
	return u.switchProviderLocked(ctx, chatID, last.ProviderID)
}

func (u *runnerUsecase) StopChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason every teardown path takes it: a stop
	// racing a switch or resume must not terminate a runner the other path is mid-way
	// through placing. It is never taken on the hook path, so a CLI still talking as it
	// dies can always reach us.
	defer u.spawns.lock(chatID)()

	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // already dormant: there is no live CLI to stop
	}
	if err != nil {
		return fmt.Errorf("agent: stop chat: live runner: %w", err)
	}
	u.retire(ctx, live)
	return nil
}
