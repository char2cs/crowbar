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

// StartTerminalWaitSweep starts the screen sweep and wires the three publish
// callbacks the hub owns: promptSettled here, messageDelta and
// compactionStatus onto the hook ingress.

// All three are assigned BEFORE the nil-detector return. A daemon with no
// detector still streams assistant messages (and compaction status) to its
// chat UI, and dropping either on that path is invisible until a user
// watches a message that never grows, or a compaction that never shows.
func (rs *Runners) StartTerminalWaitSweep(
	ctx context.Context,
	publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
	promptSettled func(chatID, workspaceID, requestID string),
	messageDelta func(chatID, workspaceID, messageID, text string),
	compactionStatus func(chatID, workspaceID string, active bool),
) {
	rs.promptSettled = promptSettled
	rs.turns.SetMessageDelta(messageDelta)
	rs.turns.SetCompactionStatus(compactionStatus)
	if rs.termWait == nil {
		return
	}
	rs.termWait.Run(ctx, publish)
}
