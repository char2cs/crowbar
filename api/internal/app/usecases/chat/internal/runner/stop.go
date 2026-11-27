package runner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) StopChat(
	ctx context.Context,
	chatID string,
) error {
	return rs.endRunner(ctx, chatID, true)
}

func (rs *Runners) KillChat(
	ctx context.Context,
	chatID string,
) error {
	return rs.endRunner(context.WithoutCancel(ctx), chatID, false)
}

func (rs *Runners) endRunner(
	ctx context.Context,
	chatID string,
	gentle bool,
) error {
	defer rs.parks.preempt(chatID)()
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseStopping)()
	for {
		unlock, err := rs.chatLock.Lock(ctx, chatID)
		if err != nil {
			return fmt.Errorf("agent: stop chat: %w", err)
		}
		v, ok := rs.live.view(chatID)
		if !ok {
			err := rs.settleStaleWork(ctx, chatID)
			unlock()
			return err
		}
		if v.exiting {
			unlock()
			select {
			case <-v.exited:
				continue
			case <-ctx.Done():
				return fmt.Errorf("agent: stop chat: %w", ctx.Err())
			}
		}
		retiring, err := rs.stopRunner(ctx, chatID, v.runner, gentle)
		unlock()
		if err != nil || !retiring {
			return err
		}
		if err := rs.endMarked(ctx, v.runner); err != nil {
			return fmt.Errorf("agent: stop chat: %w", err)
		}
		return nil
	}
}

func (rs *Runners) stopRunner(ctx context.Context, chatID string, live agents.Runner, gentle bool) (bool, error) {
	stopped := false
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		turn, running := st.RunningTurn()
		stopped = running
		rows, err := ledger.Stop(st, rs.turns.LiveDiff(chatID, live.ID))
		if err != nil {
			return nil, err
		}
		cancelled, err := cancelPendingMove(st, "stopped")
		if rows = append(rows, cancelled...); err != nil || !running {
			return rows, err
		}
		divider, err := ledger.NewRow(ledger.KindMarker, turn.ID, "", ledger.Marker{Kind: ledger.MarkerStopped})
		return append(rows, divider), err
	})
	if err != nil {
		return false, fmt.Errorf("agent: stop chat: %w", err)
	}
	if gentle && stopped && rs.interruptTurn(ctx, live) {
		return false, nil
	}
	_, retiring := rs.markEnding(live, domain.AgentExitStopped)
	return retiring, nil
}

func (rs *Runners) settleStaleWork(ctx context.Context, chatID string) error {
	if _, live := rs.live.byChat(chatID); live {
		return nil
	}
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		if !st.Busy() {
			return nil, nil
		}
		return ledger.Stop(st, "")
	})
	if err != nil {
		return fmt.Errorf("agent: settle stale work: %w", err)
	}
	return nil
}

const interruptEvent = "interrupt"

func (rs *Runners) interruptTurn(ctx context.Context, live agents.Runner) bool {
	conn, ok := rs.apiConns.get(live.ID)
	if !ok {
		return false
	}
	crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, live.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: interrupt turn: worktree dir (falling back to a full stop)",
			"runner_id", live.ID, "err", err)
		return false
	}
	agent, err := rs.agents.Get(ctx, crowbarHome, live.ProviderID)
	if err != nil {
		slog.WarnContext(ctx, "agent: interrupt turn: resolve descriptor (falling back to a full stop)",
			"runner_id", live.ID, "err", err)
		return false
	}
	wire, _, ok := agent.OutboundCall(interruptEvent, nil)
	if !ok || wire == "prompt" {
		return false
	}
	rs.background.run(ctx, func(ctx context.Context) {
		if err := conn.driver.Send(ctx, interruptEvent, nil); err != nil {
			slog.WarnContext(ctx, "agent: interrupt turn: send", "runner_id", live.ID, "err", err)
		}
	})
	return true
}

func (rs *Runners) divide(ctx context.Context, chatID, kind, detail string) {
	row, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: kind, Detail: detail})
	if err == nil {
		_, _, err = rs.ledger.Append(context.WithoutCancel(ctx), chatID, row)
	}
	if err != nil {
		slog.WarnContext(ctx, "agent: record divider (best-effort, continuing)", "chat_id", chatID, "kind", kind, "err", err)
	}
}
