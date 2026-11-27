package runner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) exitRows(
	st ledger.State, chatID string, e ledger.RunnerExited, reason string,
) ([]ledger.Row, error) {
	salvaged, err := rs.turns.ExitRows(st, chatID, e.RunnerID)
	if err != nil {
		return nil, err
	}
	ended, err := ledger.Exit(st, e, reason)
	return append(salvaged, ended...), err
}

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
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: boot reconcile: chat %s: %w", chatID, err)
	}
	dead, ok := st.LiveRunner()
	if !ok {
		return rs.settleStaleWork(ctx, chatID)
	}
	runner := agents.Runner{ID: dead.RunnerID, ProviderID: dead.Provider, CurrentChatID: chatID}
	rs.reapCrashOrphanRunnerTmp(ctx, runner)
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
	return rs.settleStaleWork(ctx, chatID)
}

func (rs *Runners) Shutdown() {
	rs.live.exitAll(domain.AgentExitDaemonRestart)
	rs.background.stop()
	rs.apiConns.closeAll()
}

func (rs *Runners) reconcileRunnerExit(ctx context.Context, runnerID string) string {
	ctx = context.WithoutCancel(ctx)
	rs.agents.ForgetRunner(runnerID)
	rs.surfaces.drop(runnerID)
	rs.answers.ReleaseRunner(runnerID)
	entry, ok := rs.claimEnd(runnerID)
	if !ok {
		return ""
	}
	defer rs.live.remove(runnerID)
	if !entry.started {
		return ""
	}
	chatID := entry.runner.CurrentChatID
	defer rs.touch(ctx, chatID)
	lost := rs.loseRefusedSession(ctx, runnerID, entry.exit)
	reason := cmp.Or(entry.cause, lost, domain.AgentExitExited)
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		return rs.exitRows(st, chatID, ledger.RunnerExited{RunnerID: runnerID, Reason: reason, ExitCode: entry.exit.Code},
			ledger.ReasonRunnerExited)
	})
	if err != nil {
		slog.ErrorContext(ctx, "agent: record runner exit", "chat_id", chatID, "runner_id", runnerID, "err", err)
	}
	rs.turns.Retire(chatID, runnerID)
	slog.InfoContext(ctx, "agent: runner ended", "component", "runner", "chat", chatID, "reason", reason)
	return reason
}

func (rs *Runners) endUnheld(ctx context.Context, runnerID string) {
	ctx = context.WithoutCancel(ctx)
	runner, ok := rs.live.byRunner(runnerID)
	if !ok {
		rs.reconcileRunnerExit(ctx, runnerID)
		return
	}
	unlock, err := rs.chatLock.Lock(ctx, runner.CurrentChatID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: record runner exit: hold the chat", "runner_id", runnerID, "err", err)
		return
	}
	defer unlock()
	rs.reconcileRunnerExit(ctx, runnerID)
}

func (rs *Runners) onRunnerExit(home, runnerID, tmpDir string, resume resumeWatch) func(engineterminal.Exit) {
	return func(exit engineterminal.Exit) {
		ctx := context.Background()
		rs.reapRunnerDir(ctx, home, tmpDir)
		rs.apiConns.drop(runnerID)
		rs.live.noteExit(runnerID, runnerExit{Exit: exit, resume: resume})
		if rs.live.exitBeforeStart(runnerID) {
			return
		}
		rs.endUnheld(ctx, runnerID)
	}
}

func (rs *Runners) reapRunnerDir(
	ctx context.Context,
	home string,
	dir string,
) {
	rs.runnerDirs.Lock()
	defer rs.runnerDirs.Unlock()
	if err := worktreepath.RemoveUnderHome(home, dir); err != nil {
		slog.WarnContext(ctx, "agent: reap a runner directory (best-effort)", "err", err)
		return
	}
	for _, parent := range []string{filepath.Dir(dir), filepath.Dir(filepath.Dir(dir))} {
		if !worktreepath.UnderHome(parent, home) || os.Remove(parent) != nil {
			return
		}
	}
}

func (rs *Runners) RetireChatRunners(
	ctx context.Context,
	chatID string,
) (end func()) {
	defer rs.turns.ForgetChat(chatID)
	live, ok := rs.live.byChat(chatID)
	if !ok {
		return func() {}
	}
	rs.markEnding(live, domain.AgentExitDisplaced)
	rs.reconcileRunnerExit(ctx, live.ID)
	return func() {
		if _, err := rs.endProcesses(ctx, live); err != nil {
			slog.WarnContext(ctx, "agent: retire the chat's runner (best-effort, continuing)",
				"chat_id", chatID, "runner_id", live.ID, "err", err)
		}
	}
}

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
	rs.reapRunnerDir(ctx, home, worktreepath.RunnerDir(chatsDir, runner.ID, runner.ProviderID))
}
