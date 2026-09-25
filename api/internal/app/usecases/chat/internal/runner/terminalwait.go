package runner

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (rs *Runners) TerminalWait(chatID string) domain.AgentTerminalWait {
	if rs.termWait == nil {
		return domain.AgentTerminalWait{}
	}
	return rs.termWait.Wait(chatID)
}

// StartTerminalWaitSweep starts the screen sweep and binds the live chat feed
// the hub owns.
//
// The feed is bound BEFORE the nil-detector return. A daemon with no detector
// still streams assistant messages, compaction status, plans and usage to its
// chat UI, and dropping any of them on that path is invisible until a user
// watches a message that never grows.
func (rs *Runners) StartTerminalWaitSweep(ctx context.Context, feed seam.ChatFeed) {
	rs.promptSettled = feed.PromptSettled
	rs.turns.SetFeed(feed)
	if rs.termWait == nil {
		return
	}
	// The verdict rides the chat snapshot: a change republishes it.
	rs.termWait.Run(ctx, func(chatID, _ string, _ domain.AgentTerminalWait) {
		rs.touch(ctx, chatID)
	})
}
