package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// StopChat, and the one owner of what a Stop records: stopRunner.
func (rs *Runners) StopChat(
	ctx context.Context,
	chatID string,
) error {
	// The chat's spawn gate, for the same reason every teardown path takes it: a stop
	// racing a switch or resume must not terminate a runner the other path is mid-way
	// through placing. PREEMPTED, not queued for: a switch or resume parked on the
	// outgoing turn (bounded by minutes) abandons its wait with nothing changed, so
	// Stop is bounded by the teardown alone (invariant A2). It is never taken on the
	// hook path, so a CLI still talking as it dies can always reach us.
	release, err := rs.spawns.Preempt(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: stop chat: %w", err)
	}
	defer release()
	defer rs.enterPhase(ctx, chatID, snapshot.PhaseStopping)()

	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // already dormant: there is no live CLI to stop
	}
	if err != nil {
		return fmt.Errorf("agent: stop chat: live runner: %w", err)
	}
	// Mid-turn, an api provider is interrupted in place (its session and
	// connection survive); otherwise — idle, hooks-only, or an interrupt that
	// does not land in time — the runner is retired.
	rs.stopRunner(ctx, chatID, live, true)
	return nil
}

// stopRunner ends live's work on chatID — in place when gentle and the
// provider can interrupt a turn over its api connection, otherwise by retiring
// it — and records exactly one "stopped" interruption when a turn was open.
//
// Whether a turn was open is read BEFORE either teardown runs, and is the only
// input to the record. Both teardowns end the turn themselves — displace
// completes the in-flight entry, and an interrupt races the CLI's own
// turn_stop — so asking afterwards answered "idle" on every path and the
// divider was never written (P0-8).
//
// The record comes AFTER the teardown, never before: interruptTurn's Send
// blocks until the CLI's turn has genuinely ended, and retire's kill is
// synchronous, so the divider lands after the last content the CLI produced
// rather than ahead of it.
func (rs *Runners) stopRunner(ctx context.Context, chatID string, live agents.Runner, gentle bool) {
	open, err := rs.turns.TurnOpen(ctx, chatID, live.ID)
	if err != nil {
		slog.WarnContext(ctx, "agent: stop: read turn state (assuming idle)", "chat_id", chatID, "err", err)
	}
	if !gentle || !open || !rs.interruptTurn(ctx, live) {
		rs.retire(ctx, live)
		rs.noteChatExit(ctx, chatID, domain.AgentExitStopped)
	}
	if !open {
		return
	}
	if err := rs.turns.RecordStop(ctx, chatID, live.ID); err != nil {
		slog.WarnContext(ctx, "agent: stop: record interruption", "chat_id", chatID, "err", err)
	}
}

// interruptEvent is the canonical outbound event a provider declares when
// Crowbar can cancel its in-flight turn without ending the session — the one
// case StopChat's full teardown (retire) is too blunt for. Key-presence on the
// descriptor is the whole capability check, same as compactStartEvent.
const interruptEvent = "interrupt"

// defaultInterruptBound is how long Stop waits for an in-place interrupt to
// land before it retires the runner instead.
const defaultInterruptBound = 10 * time.Second

func (rs *Runners) interruptBound() time.Duration {
	if rs.interruptTimeout > 0 {
		return rs.interruptTimeout
	}
	return defaultInterruptBound
}

// interruptTurn asks a live api-transport connection to cancel its current
// turn in place, and reports whether it actually did: false for anything that
// falls back to StopChat's own teardown — no live api driver, a descriptor
// that declares no interrupt gesture, or one whose gesture isn't reachable
// over this connection (wire == "prompt", the same refusal Compact makes).
// That fallback is today's behaviour for every provider, unchanged; this only
// adds a better path where one now exists.
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
	// Bounded: the provider may defer its reply until the turn actually ends,
	// and a wedged turn must not hold Stop (and the preempted gate) with it.
	sendCtx, cancel := context.WithTimeout(ctx, rs.interruptBound())
	defer cancel()
	if err := conn.driver.Send(sendCtx, interruptEvent, nil); err != nil {
		slog.WarnContext(ctx, "agent: interrupt turn: send (falling back to a full stop)",
			"runner_id", live.ID, "err", err)
		return false
	}
	return true
}
