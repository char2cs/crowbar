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
	reason := conn.lossReason()
	slog.WarnContext(context.WithoutCancel(ctx), "agent: api transport: connection lost",
		"runner_id", runnerID, "reason", reason)
	conn.lost.Store(&reason)
	rs.apiConns.drop(runnerID)
}

// lossReason names c dying on its own.
func (c *apiconn) lossReason() string {
	if c.driver != nil && c.driver.Overflowed() {
		return domain.AgentExitTransportOverflow
	}
	return domain.AgentExitConnectionLost
}

// exitCause is why c's serve exited: "" when Crowbar took it down. drop cancels
// c.ctx before it signals the process, so a live ctx means it died on its own.
func (c *apiconn) exitCause() string {
	if c.ctx != nil && c.ctx.Err() == nil {
		return c.lossReason()
	}
	return c.lostReason() // after ctx: onAPIConnLost stores it before it cancels
}
