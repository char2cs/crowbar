package runner

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (rs *Runners) TerminalWait(chatID string) domain.AgentTerminalWait {
	if rs.termWait == nil {
		return domain.AgentTerminalWait{}
	}
	return rs.termWait.Wait(chatID)
}

// StartTerminalWaitSweep starts the screen sweep, when the terminal seam can
// render a screen.
func (rs *Runners) StartTerminalWaitSweep(ctx context.Context) {
	if rs.termWait == nil {
		return
	}
	// The verdict rides the chat snapshot: a change republishes it.
	rs.termWait.Run(ctx, func(chatID, _ string, _ domain.AgentTerminalWait) {
		rs.touch(ctx, chatID)
	})
}
