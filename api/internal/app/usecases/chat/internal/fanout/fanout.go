// Package fanout turns repository lifecycle announcements into chat snapshots.
//
// The repositories announce WHAT HAPPENED, carrying the aggregate as of each
// event; this package hands both aggregates' events to the one snapshot owner
// (internal/snapshot), which versions them and publishes the frame. That is
// what makes "one lifecycle change → exactly one versioned frame" a property
// you can point at.
package fanout

import (
	"context"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
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
		f.snaps.ApplyChat(context.Background(), chat, e.Version, e.Kind, e.Forgotten)
	}
}

// RunnerWatch is the seam agentrunner.NewEventSourced is wired with.
//
// It is never nil: agentrunner's store REFUSES a nil watch at construction.
func (f *Fanout) RunnerWatch() agentrunner.WatchFunc {
	return func(e agentrunner.RunnerEvent) {
		runner := e.Runner
		runner.ID = e.RunnerID
		f.snaps.ApplyRunner(context.Background(), runner, e.Previous, e.Version, e.Kind)
	}
}
