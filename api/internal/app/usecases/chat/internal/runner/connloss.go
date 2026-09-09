// Package runner (file connloss.go) reconciles a chat after its api-transport
// connection dies on its own — as opposed to the four paths where Crowbar takes
// one down deliberately (retire, provider switch, PTY exit, SwitchToTerminal),
// each of which already owns the turn it was holding.
//
// This is a separate concern from apiconn.go's job of establishing and pumping a
// connection: that file is about a connection that works, this one is about what
// a chat is left holding when one stops.
package runner

import (
	"context"
	"log/slog"
)

// onAPIConnLost tears down after a connection that died on its own.
//
// The two cases are told apart by the connection's OWN ctx: drop cancels it, and
// every deliberate teardown goes through drop. Each of those callers already
// closes whatever turn was open, so reconciling again here would close a turn a
// successor runner had legitimately opened in the meantime.
//
// An unexpected loss is reconciled the same way a dead CLI is, because for an
// api-transport provider that is what it means: the connection IS the
// conversation, and the companion PTY that outlives it is driving a different,
// unrelated one.
//
// Forgetting the registry entry FIRST is the load-bearing half. While it stood,
// HasLiveAPIConnection answered true forever and apiOwnsThisEvent (turn's
// ingest.go) went on discarding the companion PTY's hooks copy of every
// api-owned event as a redundant duplicate of a transport that no longer
// existed — so the chat fell permanently silent on top of spinning forever.
func (rs *Runners) onAPIConnLost(ctx context.Context, runnerID string) {
	if ctx.Err() != nil {
		return // deliberate teardown; the caller that dropped it owns the turn
	}
	rs.apiConns.drop(runnerID)

	if rs.runnerStore == nil {
		return
	}
	// ctx belongs to the connection that just died, so the reconcile gets its
	// own — anything derived from the dead one is already cancelled.
	reconcileCtx := context.Background()
	runner, err := rs.runnerStore.Get(reconcileCtx, runnerID)
	if err != nil {
		return // already forgotten: its own exit path ran and owns the turn
	}
	slog.WarnContext(reconcileCtx, "agent: api transport: connection lost, reconciling its turn",
		"runner_id", runnerID, "chat_id", runner.CurrentChatID)
	// Release anyone blocked on this runner's turn before closing it, the same
	// order displace uses: a waiter left holding a turn nothing will ever finish
	// waits out its own context instead.
	if rs.inflightTurns != nil {
		rs.inflightTurns.Complete(runnerID)
	}
	rs.closeAbandonedTurn(reconcileCtx, runner.CurrentChatID, runner)
}
