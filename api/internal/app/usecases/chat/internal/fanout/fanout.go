package fanout

import (
	"context"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
)

type Fanout struct {
	snaps *snapshot.Snapshots
}

func New(snaps *snapshot.Snapshots) *Fanout { return &Fanout{snaps: snaps} }

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
