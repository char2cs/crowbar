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

func (rs *Runners) quitRunner(ctx context.Context, runner agents.Runner, cause string) error {
	exited, err := rs.retire(ctx, runner, cause)
	if err != nil || exited == nil {
		return err
	}
	select {
	case <-exited:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("agent: wait for runner %s to exit: %w", runner.ID, ctx.Err())
	}
}

func (rs *Runners) retire(ctx context.Context, runner agents.Runner, cause string) (<-chan struct{}, error) {
	exited, ok := rs.markEnding(runner, cause)
	if !ok {
		return nil, nil
	}
	if err := rs.endMarked(ctx, runner); err != nil {
		return nil, err
	}
	return exited, nil
}

func (rs *Runners) endMarked(ctx context.Context, runner agents.Runner) error {
	ptyExits, err := rs.endProcesses(ctx, runner)
	if err != nil {
		rs.live.keepLive(runner.ID)
		return err
	}
	if !ptyExits {
		rs.endUnheld(ctx, runner.ID)
	}
	return nil
}

func (rs *Runners) markOutgoingEnding(chatID string) {
	if live, ok := rs.live.byChat(chatID); ok {
		rs.live.markExiting(live.ID, domain.AgentExitDisplaced)
	}
}

func (rs *Runners) quitOutgoingCLI(ctx context.Context, chatID string) error {
	live, ok := rs.live.byChat(chatID)
	if !ok {
		return nil
	}
	if err := rs.quitRunner(ctx, live, domain.AgentExitDisplaced); err != nil {
		return fmt.Errorf("agent: switch provider: %w", err)
	}
	return nil
}

func (rs *Runners) markEnding(runner agents.Runner, cause string) (exited <-chan struct{}, ok bool) {
	rs.live.inTurnOrder(runner.CurrentChatID, func() {
		exited, ok = rs.live.markExiting(runner.ID, cause)
	})
	return exited, ok
}

func (rs *Runners) claimEnd(runnerID string) (claimed liveEntry, ok bool) {
	runner, ok := rs.live.byRunner(runnerID)
	if !ok {
		return liveEntry{}, false
	}
	rs.live.inTurnOrder(runner.CurrentChatID, func() {
		claimed, ok = rs.live.claimExit(runnerID)
	})
	return claimed, ok
}
