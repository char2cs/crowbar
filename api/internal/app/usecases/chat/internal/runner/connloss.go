// Package runner (file connloss.go) handles an api connection that dies on
// its own, as opposed to one Crowbar takes down (every deliberate teardown
// cancels the connection's ctx first).
package runner

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// onAPIConnLost ends a runner whose api channel is gone. The connection IS
// the runner's channel, so it is not left running without one: the serve
// process is killed and its exit watcher reconciles the runner (turn closed,
// row exited) through the one exit path, with the loss recorded as the cause.
func (rs *Runners) onAPIConnLost(ctx context.Context, runnerID string) {
	if ctx.Err() != nil {
		return // deliberate teardown; the caller that dropped it owns the rest
	}
	reason := domain.AgentExitConnectionLost
	if conn, ok := rs.apiConns.get(runnerID); ok && conn.driver.Overflowed() {
		reason = domain.AgentExitTransportOverflow
	}
	slog.WarnContext(context.WithoutCancel(ctx), "agent: api transport: connection lost; ending its runner",
		"runner_id", runnerID, "reason", reason)
	rs.sessions.cause(runnerID, reason)
	rs.apiConns.drop(runnerID)
}
