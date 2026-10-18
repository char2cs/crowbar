// Package runner (file connloss.go) handles an api connection that dies on
// its own, as opposed to one Crowbar takes down (every deliberate teardown
// cancels the connection's ctx first).
package runner

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// onAPIConnLost ends a runner whose api channel is gone. A runner whose
// connection IS its process is not left running without one: the serve
// process is killed and its exit watcher records the runner's exit through
// the one exit path, with the loss as the cause. A runner with a PTY runs on.
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
	if r, ok := rs.live.byRunner(runnerID); ok && r.TerminalSession == "" {
		rs.live.noteCause(runnerID, reason)
	}
	rs.apiConns.drop(runnerID)
}
