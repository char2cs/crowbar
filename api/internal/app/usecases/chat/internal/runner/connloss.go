package runner

import (
	"context"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (rs *Runners) onAPIConnLost(ctx context.Context, conn *apiconn, runnerID string) {
	if ctx.Err() != nil {
		return
	}
	reason := conn.lossReason()
	slog.WarnContext(context.WithoutCancel(ctx), "agent: api transport: connection lost",
		"runner_id", runnerID, "reason", reason)
	conn.lost.Store(&reason)
	rs.apiConns.drop(runnerID)
}

func (c *apiconn) lossReason() string {
	if c.driver != nil && c.driver.Overflowed() {
		return domain.AgentExitTransportOverflow
	}
	return domain.AgentExitConnectionLost
}

func (c *apiconn) exitCause() string {
	if c.ctx != nil && c.ctx.Err() == nil {
		return c.lossReason()
	}
	return c.lostReason()
}
