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
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// endProcesses ends runner's processes. The PTY goes first and its failure is
// returned before anything else is touched, so a caller that must not proceed
// on a still-running CLI (a switch) aborts with nothing changed.
func (rs *Runners) endProcesses(ctx context.Context, runner agents.Runner) error {
	if runner.TerminalSession != "" {
		err := rs.term.TerminateGraceful(ctx, runner.TerminalSession)
		if err != nil && !errors.Is(err, engineterminal.ErrSessionNotFound) {
			return fmt.Errorf("terminate outgoing terminal %s: %w", runner.TerminalSession, err)
		}
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
	return nil
}

// retire takes runner off its chat for good and ends it. Best-effort: a
// runner still placed after a failed displace is reconciled by its own exit.
func (rs *Runners) retire(ctx context.Context, runner agents.Runner) {
	if err := rs.displace(ctx, runner); err != nil {
		slog.ErrorContext(ctx, "agent: retire runner: displace (best-effort, continuing)",
			"runner_id", runner.ID, "chat_id", runner.CurrentChatID, "err", err)
	}
	if runner.CurrentChatID != "" {
		rs.noteChatExit(ctx, runner.CurrentChatID, domain.AgentExitDisplaced)
	}
	if err := rs.endProcesses(ctx, runner); err != nil {
		slog.WarnContext(ctx, "agent: retire runner (best-effort, continuing)",
			"runner_id", runner.ID, "err", err)
	}
}

// quitOutgoingCLI ends chatID's live runner so a replacement can take the
// chat. Unlike retire it aborts on failure: the replacement must never land
// beside a CLI that is still running and still placed.
func (rs *Runners) quitOutgoingCLI(ctx context.Context, chatID string) error {
	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // dormant: nothing to quit
	}
	if err != nil {
		return fmt.Errorf("agent: switch provider: live runner: %w", err)
	}
	if err := rs.endProcesses(ctx, live); err != nil {
		return fmt.Errorf("agent: switch provider: %w", err)
	}
	if err := rs.displace(ctx, live); err != nil {
		return fmt.Errorf("agent: switch provider: %w", err)
	}
	rs.noteChatExit(ctx, chatID, domain.AgentExitDisplaced)
	return nil
}
