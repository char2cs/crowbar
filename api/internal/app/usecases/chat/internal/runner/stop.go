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

// StopChat answers the Stop button: the turn is interrupted in place where the
// provider allows it, and the runner is retired otherwise.
func (rs *Runners) StopChat(
	ctx context.Context,
	chatID string,
) error {
	return rs.endRunner(ctx, chatID, true)
}

// CloseChat answers a closed view: no view holds the chat any more, so its
// runner is retired even mid-turn. The chat record stays, dormant and resumable.
func (rs *Runners) CloseChat(
	ctx context.Context,
	chatID string,
) error {
	// Detached: nothing re-sends a close, so a request dropped mid-teardown
	// (app quit, socket loss) must not leave the runner live with no view.
	return rs.endRunner(context.WithoutCancel(ctx), chatID, false)
}

// endRunner is the one owner of what a Stop or Close records: stopRunner.
// Waits parked on the chat are cancelled rather than queued behind, so Stop
// is bounded by the teardown alone (invariant A2); an end already under way is
// waited out, with the chat's lock released.
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
			unlock()
			return nil // already dormant: there is no live CLI to stop
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
		// Outside the lock: the CLI's own hooks on the way down need it.
		if err := rs.endMarked(ctx, v.runner); err != nil {
			return fmt.Errorf("agent: stop chat: %w", err)
		}
		return nil
	}
}

// stopRunner ends the chat's turn and settles its pending prompts in one
// commit, records the divider when that stopped a turn, then tells live: an
// api provider mid-turn is interrupted in place, and otherwise the runner is
// marked ending (retiring) for the caller to end, its exit recorded by its own.
func (rs *Runners) stopRunner(ctx context.Context, chatID string, live agents.Runner, gentle bool) (bool, error) {
	stopped := false
	_, _, err := rs.ledger.Do(ctx, chatID, func(st ledger.State) ([]ledger.Row, error) {
		_, stopped = st.RunningTurn()
		return ledger.Stop(st)
	})
	if err != nil {
		return false, fmt.Errorf("agent: stop chat: %w", err)
	}
	if stopped {
		if err := rs.turns.RecordStop(ctx, chatID); err != nil {
			slog.WarnContext(ctx, "agent: stop: record interruption", "chat_id", chatID, "err", err)
		}
	}
	if gentle && stopped && rs.interruptTurn(ctx, live) {
		return false, nil
	}
	_, retiring := rs.markEnding(live, domain.AgentExitStopped)
	return retiring, nil
}

// interruptEvent is the canonical outbound event a provider declares when
// Crowbar can cancel its in-flight turn without ending the session — the one
// case StopChat's full teardown (retire) is too blunt for. Key-presence on the
// descriptor is the whole capability check, same as compactStartEvent.
const interruptEvent = "interrupt"

// interruptTurn asks a live api connection to cancel its current turn in
// place, and reports whether it can: false — no api driver, no interrupt
// gesture, or one not reachable over this connection (wire == "prompt") —
// falls back to StopChat's retire. The send runs in the background: the
// turn is already over in the ledger, and nothing waits for the provider.
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
