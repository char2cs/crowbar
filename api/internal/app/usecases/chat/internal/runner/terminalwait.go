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

// StartTerminalWaitSweep starts the screen sweep and wires the two publish
// callbacks the hub owns: promptSettled here, messageDelta onto the hook ingress.

// Both are assigned BEFORE the nil-detector return. A daemon with no detector
// still streams assistant messages to its chat UI, and dropping messageDelta on
// that path is invisible until a user watches a message that never grows.
func (rs *Runners) StartTerminalWaitSweep(
	ctx context.Context,
	publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
	promptSettled func(chatID, workspaceID, requestID string),
	messageDelta func(chatID, workspaceID, messageID, text string),
) {
	rs.promptSettled = promptSettled
	rs.turns.SetMessageDelta(messageDelta)
	if rs.termWait == nil {
		return
	}
	rs.termWait.Run(ctx, publish)
}
