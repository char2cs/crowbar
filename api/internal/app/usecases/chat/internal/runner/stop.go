package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
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
	// ONLY WHILE THERE IS A TURN TO INTERRUPT. interruptTurn asks a live api
	// connection to cancel gracefully and leaves the CLI running — exactly what
	// the Stop button wants mid-answer (see this function's own history: killing
	// mid-turn is what corrupted a resumed session's transcript, the same
	// reasoning switchProviderLocked's awaitTurnOrForce is built around). But a
	// closed chat tab reaches this same call on an IDLE chat just as often as a
	// mid-turn one, and interruptTurn's own check has no notion of idle — a live
	// api connection plus a descriptor that declares a non-"prompt" interrupt
	// gesture (codex, always) made it return true regardless, so StopChat
	// returned having neither interrupted anything nor retired the runner.
	// Confirmed live: closing a codex tab left its runner, its api connection
	// and its companion PTY all running indefinitely, still placed on the
	// "closed" chat, directly contradicting closeBuffer's own "closing stops
	// the CLI" contract on the frontend. Gating on working restores it: an idle
	// chat always falls through to a real retire below.
	//
	// interruptTurn itself is what decides whether the CLI has actually
	// stopped: its Send blocks on the connection's reply, and codex's own
	// turn/interrupt DEFERS that reply until the turn genuinely ends (its
	// app-server only answers once TurnAborted or TurnComplete fires — see
	// codex-rs's respond_to_pending_interrupts) — so a true return here means
	// the turn is over, not merely asked to be. retire's kill is synchronous
	// for the same reason. RecordStop is called AFTER, never before: it used
	// to fire the instant Stop was clicked, unconditionally, which durably
	// marked the turn "Interrupted" while codex kept right on generating —
	// the marker landed ahead of a full extra minute of real tool calls and
	// assistant text that arrived after it, both because the position was
	// wrong (stamped before the content it should have followed) and because
	// it was a lie (nothing had actually stopped yet). Confirmed live.
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
	open, err := rs.turnOpen(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: stop: read turn state (assuming idle)", "chat_id", chatID, "err", err)
	}
	if !gentle || !open || !rs.interruptTurn(ctx, live) {
		rs.retire(ctx, live)
	}
	if !open {
		return
	}
	if err := rs.turns.RecordStop(ctx, chatID, live.ID); err != nil {
		slog.WarnContext(ctx, "agent: stop: record interruption", "chat_id", chatID, "err", err)
	}
}

// turnOpen reports whether chatID has work a Stop would cut short: a turn in
// flight, or background work the authoritative fold still counts.
func (rs *Runners) turnOpen(ctx context.Context, chatID string) (bool, error) {
	if len(rs.inflightTurns.Inflight(chatID)) > 0 {
		return true, nil
	}
	return rs.turns.ChatWorking(ctx, chatID)
}

// interruptEvent is the canonical outbound event a provider declares when
// Crowbar can cancel its in-flight turn without ending the session — the one
// case StopChat's full teardown (retire) is too blunt for. Key-presence on the
// descriptor is the whole capability check, same as compactStartEvent.
const interruptEvent = "interrupt"

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
	if err := conn.driver.Send(ctx, interruptEvent, nil); err != nil {
		slog.WarnContext(ctx, "agent: interrupt turn: send (falling back to a full stop)",
			"runner_id", live.ID, "err", err)
		return false
	}
	return true
}
