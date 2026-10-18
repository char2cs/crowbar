package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Frame kinds the runner lifecycle announces on its chat.
const (
	frameStarted = "started"
	frameExited  = "exited"
)

// closeAbandonedTurn closes the turn runner left open on chatID. No successor
// can hold the chat yet: a runner leaves the live table only after this.
func (rs *Runners) closeAbandonedTurn(
	ctx context.Context,
	chatID string,
	runner agents.Runner,
) {
	if chatID == "" {
		return
	}
	// Salvage what this runner already streamed first: the text was broadcast
	// live, and a turn closed without it leaves the client's bubble open.
	rs.salvageStreamed(ctx, chatID, runner)
	rs.abandonTurn(ctx, chatID)
}

func (rs *Runners) salvageStreamed(ctx context.Context, chatID string, runner agents.Runner) {
	if rs.turns == nil {
		return
	}
	if _, err := rs.turns.AbandonMessageForRunner(ctx, chatID, runner); err != nil {
		slog.WarnContext(ctx, "agent: close abandoned turn: salvage streamed message (best-effort, continuing)",
			"chat_id", chatID, "runner_id", runner.ID, "err", err)
	}
}

// abandonTurn closes chatID's open turn, whatever its CLI last reported.
func (rs *Runners) abandonTurn(ctx context.Context, chatID string) {
	// AbandonTurn, not StopTurn: a dead CLI never restates the async work it
	// last reported, and Working folds from that level too. The conversation
	// record goes first, or its open tool calls render as running forever.
	if err := rs.activity.Abandon(ctx, chatID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close abandoned turn: abandon conversation record",
			"chat_id", chatID, "err", err)
	}

	abandoned, err := rs.chats.AbandonTurn(ctx, chatID, time.Now())
	if err == nil {
		rs.work.Set(chatID, abandoned.Working)
		return
	}
	if errors.Is(err, asynxModels.ErrValidation) {
		// The command's authoritative fold says there is nothing to abandon.
		rs.work.Set(chatID, false)
		return
	}
	slog.WarnContext(ctx, "agent: close abandoned turn: abandon turn", "chat_id", chatID, "err", err)
}

// ReconcileRunnersOnBoot ends, as daemon_restart, every runner the ledger
// still holds live: no CLI outlives the daemon that started it. The pending
// prompt pass runs whatever the runner pass met.
func (rs *Runners) ReconcileRunnersOnBoot(
	ctx context.Context,
) error {
	chats, err := rs.ledger.OpenChats(ctx)
	var errs []error
	if err != nil {
		errs = append(errs, fmt.Errorf("agent: boot reconcile: open chats: %w", err))
	}
	for _, chatID := range chats {
		if err := rs.reconcileBootRunner(ctx, chatID); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(append(errs, rs.reconcilePendingPromptsOnBoot(ctx))...)
}

func (rs *Runners) reconcileBootRunner(ctx context.Context, chatID string) error {
	var dead ledger.RunnerStarted
	rows, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		live, ok := st.LiveRunner()
		if !ok {
			return nil, nil
		}
		dead = live
		return ledger.Exit(st, ledger.RunnerExited{RunnerID: live.RunnerID, Reason: domain.AgentExitDaemonRestart},
			ledger.ReasonDaemonRestart)
	})
	if err != nil {
		return fmt.Errorf("agent: boot reconcile: chat %s: %w", chatID, err)
	}
	if len(rows) == 0 {
		return nil
	}
	runner := agents.Runner{ID: dead.RunnerID, ProviderID: dead.Provider, CurrentChatID: chatID}
	rs.reapCrashOrphanRunnerTmp(ctx, runner)
	// Turn state is reconciled, never authoritative: a CLI that died mid-turn
	// sent no turn_stop, so its chat would come back spinning forever.
	rs.closeAbandonedTurn(ctx, chatID, runner)
	return nil
}

// reconcileRunnerExit records that runnerID's process is gone: one ledger
// commit names why and settles what it carried, then the chat is freed.
// Every exit signal may call it; the first to claim the runner records it.
func (rs *Runners) reconcileRunnerExit(ctx context.Context, runnerID string) {
	ctx = context.WithoutCancel(ctx)
	// What belongs to the process goes with it: the echo guard, the surface
	// mirror, relays its detached hooks still block on, and turns a switch
	// may be parked on.
	rs.agents.ForgetRunner(runnerID)
	rs.surfaces.drop(runnerID)
	rs.answers.ReleaseRunner(ctx, runnerID)
	rs.inflightTurns.Complete(ctx, runnerID, domain.AgentExitExited)

	entry, ok := rs.live.claimExit(runnerID)
	if !ok {
		return
	}
	defer rs.live.remove(runnerID)
	if !entry.started {
		return // its spawn failed, and records that itself
	}
	runner, chatID := entry.runner, entry.runner.CurrentChatID
	reason := entry.cause
	if reason == "" {
		reason = domain.AgentExitExited
	}
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Exit(st, ledger.RunnerExited{RunnerID: runnerID, Reason: reason, ExitCode: entry.code},
			ledger.ReasonRunnerExited)
	})
	if err != nil {
		slog.ErrorContext(ctx, "agent: record runner exit", "chat_id", chatID, "runner_id", runnerID, "err", err)
	}
	slog.InfoContext(ctx, "agent: runner ended", "component", "runner", "chat", chatID, "reason", reason)
	rs.announceRunner(ctx, chatID, runnerID, frameExited)
	rs.closeAbandonedTurn(ctx, chatID, runner)
}

// onRunnerExit is a runner process's exit callback.
func (rs *Runners) onRunnerExit(home, runnerID, tmpDir string, resume resumeWatch) func(engineterminal.Exit) {
	return func(exit engineterminal.Exit) {
		ctx := context.Background()
		rs.loseRefusedSession(ctx, resume, runnerID, exit)
		reapRunnerDir(ctx, home, tmpDir)
		// A dead PTY takes its api connection with it.
		rs.apiConns.drop(runnerID)
		rs.live.noteExitCode(runnerID, exit.Code)
		// A process that died before its runner_started committed is recorded by
		// spawnRunner, once the hooks it emitted have been replayed in order.
		if rs.pendingHooks.MarkExited(runnerID) {
			return
		}
		rs.reconcileRunnerExit(ctx, runnerID)
	}
}

// reapRunnerDir deletes a runner's own directory, best-effort: a reap that
// cannot happen leaves disk behind, which must not fail the runner's teardown.
func reapRunnerDir(
	ctx context.Context,
	home string,
	dir string,
) {
	if err := worktreepath.RemoveUnderHome(home, dir); err != nil {
		slog.WarnContext(ctx, "agent: reap a runner directory (best-effort)", "err", err)
	}
}

// RetireChatRunners ends chatID's runner and forgets everything held in
// memory for the chat: it is being erased (A7).
func (rs *Runners) RetireChatRunners(
	ctx context.Context,
	chatID string,
) {
	if live, ok := rs.live.byChat(chatID); ok {
		if err := rs.quitRunner(ctx, live, domain.AgentExitDisplaced); err != nil {
			slog.WarnContext(ctx, "agent: retire the chat's runner (best-effort, continuing)",
				"chat_id", chatID, "runner_id", live.ID, "err", err)
		}
	}
	rs.work.Forget(chatID)
	rs.inflightTurns.Forget(chatID)
	if rs.turns != nil {
		rs.turns.ForgetChat(chatID)
	}
}

// reapCrashOrphanRunnerTmp removes the directory of a runner whose exit
// callback never ran, found where its spawn put it.
func (rs *Runners) reapCrashOrphanRunnerTmp(
	ctx context.Context,
	runner agents.Runner,
) {
	chat, err := rs.chats.GetChat(ctx, runner.CurrentChatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: chat (best-effort, continuing)",
			"runner_id", runner.ID, "chat_id", runner.CurrentChatID, "err", err)
		return
	}
	workspaceID, err := rs.cwdWorkspaceID(ctx, chat.ID, chat.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: workspace (best-effort, continuing)",
			"runner_id", runner.ID, "chat_id", chat.ID, "err", err)
		return
	}
	chatsDir, err := rs.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: chats dir (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", workspaceID, "err", err)
		return
	}
	home, _, _, _, err := rs.ws.WorktreeDir(ctx, workspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: home (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", workspaceID, "err", err)
		return
	}
	reapRunnerDir(ctx, home, worktreepath.RunnerDir(chatsDir, runner.ID, runner.ProviderID))
}
