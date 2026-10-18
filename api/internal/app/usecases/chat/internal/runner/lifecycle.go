package runner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	asynxModels "github.com/char2cs/asynx/models"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
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

// closeAbandonedTurn closes the turn runner left open on chatID. An error
// means the turn may still be open, so the caller must not record the exit:
// the ledger then keeps the runner live, and the next boot closes both.
func (rs *Runners) closeAbandonedTurn(
	ctx context.Context,
	chatID string,
	runner agents.Runner,
) error {
	if chatID == "" {
		return nil
	}
	rs.salvageStreamed(ctx, chatID, runner)
	return rs.abandonTurn(ctx, chatID)
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

// abandonTurn closes chatID's open turn, whatever its CLI last reported. The
// conversation record goes first, or its open tool calls render as running.
func (rs *Runners) abandonTurn(ctx context.Context, chatID string) error {
	var errs []error
	if err := rs.activity.Abandon(ctx, chatID, time.Now()); err != nil {
		errs = append(errs, fmt.Errorf("agent: abandon conversation record: %w", err))
	}
	abandoned, err := rs.chats.AbandonTurn(ctx, chatID, time.Now())
	switch {
	case err == nil:
		rs.work.Set(chatID, abandoned.Working)
	case errors.Is(err, asynxModels.ErrValidation), errors.Is(err, agentchat.ErrNotFound):
		rs.work.Set(chatID, false) // no turn is open
	default:
		errs = append(errs, fmt.Errorf("agent: abandon turn: %w", err))
	}
	return errors.Join(errs...)
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

// AdoptLegacyRunners records each CLI the pre-ledger daemon left running as
// its chat's live runner, for the boot reconcile to end and close the turn of.
// A chat that is gone, or whose ledger already holds the runner, is skipped.
func (rs *Runners) AdoptLegacyRunners(ctx context.Context, runners []agents.Runner) error {
	for _, r := range runners {
		if _, err := rs.chats.GetChat(ctx, r.CurrentChatID); errors.Is(err, agentchat.ErrNotFound) {
			continue
		} else if err != nil {
			return fmt.Errorf("agent: adopt legacy runner %s: %w", r.ID, err)
		}
		row, err := ledger.NewRow(ledger.KindRunnerStarted, "", "",
			ledger.RunnerStarted{RunnerID: r.ID, Provider: r.ProviderID})
		if err == nil {
			_, _, err = rs.ledger.Do(ctx, r.CurrentChatID, func(st ledger.State) ([]ledger.Row, error) {
				_, live := st.LiveRunner()
				if exit, ok := st.LastExit(); live || ok && exit.RunnerID == r.ID {
					return nil, nil
				}
				return []ledger.Row{row}, nil
			})
		}
		if err != nil {
			return fmt.Errorf("agent: adopt legacy runner %s: %w", r.ID, err)
		}
	}
	return nil
}

func (rs *Runners) reconcileBootRunner(ctx context.Context, chatID string) error {
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile: chat %s: %w", chatID, err)
	}
	dead, ok := st.LiveRunner()
	if !ok {
		return nil
	}
	runner := agents.Runner{ID: dead.RunnerID, ProviderID: dead.Provider, CurrentChatID: chatID}
	rs.reapCrashOrphanRunnerTmp(ctx, runner)
	if err := rs.closeAbandonedTurn(ctx, chatID, runner); err != nil {
		return fmt.Errorf("agent: boot reconcile: chat %s: %w", chatID, err)
	}
	_, _, err = rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		if live, ok := st.LiveRunner(); !ok || live.RunnerID != dead.RunnerID {
			return nil, nil
		}
		return ledger.Exit(st, ledger.RunnerExited{RunnerID: dead.RunnerID, Reason: domain.AgentExitDaemonRestart},
			ledger.ReasonDaemonRestart)
	})
	if err != nil {
		return fmt.Errorf("agent: boot reconcile: chat %s: %w", chatID, err)
	}
	return nil
}

// Shutdown notes daemon_restart as how every live runner ends, then kills
// every api connection, which nothing else on the shutdown path reaches. A
// connection it detaches is recorded by the next boot.
func (rs *Runners) Shutdown() {
	rs.live.exitAll(domain.AgentExitDaemonRestart)
	rs.background.stop()
	rs.apiConns.closeAll()
}

// reconcileRunnerExit records that runnerID's process is gone: its turn is
// closed, then one ledger commit names why and settles what it carried, then
// the chat is freed. The first exit signal to claim the runner records it.
func (rs *Runners) reconcileRunnerExit(ctx context.Context, runnerID string) {
	ctx = context.WithoutCancel(ctx)
	rs.agents.ForgetRunner(runnerID)
	rs.surfaces.drop(runnerID)
	rs.answers.ReleaseRunner(ctx, runnerID)
	entry, ok := rs.claimEnd(ctx, runnerID)
	if !ok {
		return
	}
	defer rs.live.remove(runnerID)
	if !entry.started {
		return // its spawn failed, and records that itself
	}
	chatID := entry.runner.CurrentChatID
	defer rs.announceRunner(ctx, chatID, runnerID, frameExited)
	if err := rs.closeAbandonedTurn(ctx, chatID, entry.runner); err != nil {
		slog.ErrorContext(ctx, "agent: close the turn of an ended runner; the next boot records its exit",
			"chat_id", chatID, "runner_id", runnerID, "err", err)
		rs.settleCarriedPrompts(ctx, chatID, runnerID)
		return
	}
	reason := cmp.Or(entry.cause, domain.AgentExitExited)
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.Exit(st, ledger.RunnerExited{RunnerID: runnerID, Reason: reason, ExitCode: entry.code},
			ledger.ReasonRunnerExited)
	})
	if err != nil {
		slog.ErrorContext(ctx, "agent: record runner exit", "chat_id", chatID, "runner_id", runnerID, "err", err)
	}
	slog.InfoContext(ctx, "agent: runner ended", "component", "runner", "chat", chatID, "reason", reason)
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
