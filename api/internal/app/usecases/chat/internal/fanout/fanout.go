// Package fanout turns repository lifecycle announcements, each carrying the aggregate as of it,
// into chat snapshots through the one snapshot owner: one lifecycle change, one versioned frame.
package fanout

import (
	"context"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
)

// Fanout feeds the snapshot owner. A nil owner degrades to a no-op so a
// daemon wired without one (tests) never panics.
type Fanout struct {
	snaps *snapshot.Snapshots
}

func New(snaps *snapshot.Snapshots) *Fanout { return &Fanout{snaps: snaps} }

// ChatWatch is the seam agentchat.NewEventSourced is wired with.
func (f *Fanout) ChatWatch() agentchat.WatchFunc {
	return func(e agentchat.ChatEvent) {
		chat := e.Chat
		chat.ID = e.ChatID
		if chat.WorkspaceID == "" {
			chat.WorkspaceID = e.WorkspaceID
		}
		f.snaps.ApplyChat(context.Background(), chat, e.Version, e.Forgotten)
	}
}
