// Package runner (file processes.go) ends a runner's processes. A runner is
// one channel at a time — its PTY, the native view it was handed over to, or
// its api connection's serve process — and every path that takes a runner off
// its chat ends all of them through here, so none is ever leaked.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// endProcesses ends runner's processes and reports whether its PTY's own exit
// callback is still to record the end. The PTY goes first and its failure is
// returned before anything else is touched, so a caller that must not proceed
// on a still-running CLI (a switch) aborts with nothing changed.
func (rs *Runners) endProcesses(ctx context.Context, runner agents.Runner) (bool, error) {
	ptyExits := false
	if runner.TerminalSession != "" {
		err := rs.term.TerminateGraceful(ctx, runner.TerminalSession)
		if err != nil && !errors.Is(err, engineterminal.ErrSessionNotFound) {
			return false, fmt.Errorf("terminate outgoing terminal %s: %w", runner.TerminalSession, err)
		}
		ptyExits = err == nil
	}
	if view, ok := rs.attached.get(runner.ID); ok {
		rs.attached.drop(runner.ID)
		if err := rs.term.TerminateGraceful(ctx, view.termSessID); err != nil &&
			!errors.Is(err, engineterminal.ErrSessionNotFound) {
			slog.WarnContext(ctx, "agent: end runner: terminate native view (best-effort, continuing)",
				"runner_id", runner.ID, "terminal_session_id", view.termSessID, "err", err)
		}
	}
	rs.apiConns.drop(runner.ID)
	return ptyExits, nil
}

// quitRunner ends runner for cause and returns once its exit is recorded, so
// a replacement never lands beside it. The wait is on the exit itself, never
// a clock. A runner with no PTY of its own is recorded here: the native view
// it may have been was its last process, and its exit handler needs the gate
// the caller holds.
func (rs *Runners) quitRunner(ctx context.Context, runner agents.Runner, cause string) error {
	exited, ok := rs.live.markExiting(runner.ID, cause)
	if !ok {
		return nil // already gone
	}
	ptyExits, err := rs.endProcesses(ctx, runner)
	if err != nil {
		rs.live.keepLive(runner.ID)
		return err
	}
	if !ptyExits {
		rs.reconcileRunnerExit(ctx, runner.ID)
	}
	select {
	case <-exited:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("agent: wait for runner %s to exit: %w", runner.ID, ctx.Err())
	}
}

// quitOutgoingCLI ends chatID's live runner so a replacement can take the
// chat, and aborts on failure: the replacement must never land beside a CLI
// that is still running.
func (rs *Runners) quitOutgoingCLI(ctx context.Context, chatID string) error {
	live, ok := rs.live.byChat(chatID)
	if !ok {
		return nil // dormant: nothing to quit
	}
	if err := rs.quitRunner(ctx, live, domain.AgentExitDisplaced); err != nil {
		return fmt.Errorf("agent: switch provider: %w", err)
	}
	return nil
}
