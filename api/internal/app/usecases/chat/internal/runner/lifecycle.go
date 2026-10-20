package runner

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

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

// closeAbandonedTurn closes, in the conversation record, the turn runner left
// open on chatID: its streamed text is kept and its open tool calls end. The
// ledger closes the turn itself in the runner's exit commit.
func (rs *Runners) closeAbandonedTurn(
	ctx context.Context,
	chatID string,
	runner agents.Runner,
) {
	if chatID == "" {
		return
	}
	rs.salvageStreamed(ctx, chatID, runner)
	if err := rs.activity.Abandon(ctx, chatID, time.Now()); err != nil {
		slog.WarnContext(ctx, "agent: close abandoned turn: conversation record (best-effort, continuing)",
			"chat_id", chatID, "runner_id", runner.ID, "err", err)
	}
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
	rs.closeAbandonedTurn(ctx, chatID, runner)
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

// reconcileRunnerExit records that runnerID's process is gone: one ledger
// commit ends its turn, names why and settles what it carried, then the chat
// is freed. The first exit signal to claim the runner records it.
func (rs *Runners) reconcileRunnerExit(ctx context.Context, runnerID string) {
	ctx = context.WithoutCancel(ctx)
	rs.agents.ForgetRunner(runnerID)
	rs.surfaces.drop(runnerID)
	rs.answers.ReleaseRunner(ctx, runnerID)
	entry, ok := rs.claimEnd(runnerID)
	if !ok {
		return
	}
	defer rs.live.remove(runnerID)
	if !entry.started {
		return // its spawn failed, and records that itself
	}
	chatID := entry.runner.CurrentChatID
	defer rs.announceRunner(ctx, chatID, runnerID, frameExited)
	rs.closeAbandonedTurn(ctx, chatID, entry.runner)
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
		// its spawn, after that row.
		if rs.live.exitBeforeStart(runnerID) {
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

// RetireChatRunners records the end of chatID's runner and forgets the chat's
// memory: it is being erased. The caller runs end after letting the lock go, so
// a hook queued for the chat on the way down never holds up its terminate.
func (rs *Runners) RetireChatRunners(
	ctx context.Context,
	chatID string,
) (end func()) {
	if rs.turns != nil {
		defer rs.turns.ForgetChat(chatID)
	}
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
