// Package runner (file connloss.go) handles an api connection that dies on
// its own, as opposed to one Crowbar takes down (every deliberate teardown
// cancels the connection's ctx first).
package runner

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// onAPIConnLost keeps why conn died on its own, then kills its serve process.
// A runner whose process conn was ends with it, the loss as its cause; one
// with a PTY, or still being spawned, runs on.
func (rs *Runners) onAPIConnLost(ctx context.Context, conn *apiconn, runnerID string) {
	if ctx.Err() != nil {
		return // deliberate teardown; the caller that dropped it owns the rest
	}
	reason := domain.AgentExitConnectionLost
	if conn.driver.Overflowed() {
		reason = domain.AgentExitTransportOverflow
	}
	slog.WarnContext(context.WithoutCancel(ctx), "agent: api transport: connection lost",
		"runner_id", runnerID, "reason", reason)
	conn.lost.Store(&reason)
	rs.apiConns.drop(runnerID)
}
