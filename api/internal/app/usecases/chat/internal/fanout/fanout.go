// Package fanout turns repository lifecycle announcements into the frames the
// frontend receives.
//
// It exists so that deciding what a client is told lives in the usecase layer and not
// inside an asynx projection. The repositories announce WHAT HAPPENED; this package is
// the single place that shapes those announcements into wire frames, which is what
// makes "one lifecycle change → exactly one frame" a property you can point at.
package fanout

import (
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// Hub is the WS broadcaster as this package needs it. *hub.Hub satisfies it with no
// adapter — these are its own two method signatures.
type Hub interface {
	BroadcastAgentChat(chatID, workspaceID, kind string, working bool)
	BroadcastAgentRunner(runnerID, workspaceID, chatID, kind string)
}

// Fanout holds the hub the frames are sent to. A nil hub degrades to a no-op so the
// daemon never panics when wired without one (tests).
type Fanout struct {
	hub Hub
}

func New(hub Hub) *Fanout { return &Fanout{hub: hub} }

// ChatWatch is the seam agentchat.NewEventSourced is wired with.
func (f *Fanout) ChatWatch() agentchat.WatchFunc {
	return func(e agentchat.ChatEvent) {
		if f.hub == nil {
			return
		}
		// A forgotten chat is not working: it is not anything. The repository still
		// reports the aggregate's last-known Working at the moment it was forgotten,
		// so suppressing it is this layer's job.
		working := e.Working && !e.Forgotten
		f.hub.BroadcastAgentChat(e.ChatID, e.WorkspaceID, e.Kind, working)
	}
}

// RunnerWatch is the seam agentrunner.NewEventSourced is wired with.
//
// It is never nil, even with no hub: agentrunner's store REFUSES a nil watch at
// construction, so returning one would make the container fail to build.
func (f *Fanout) RunnerWatch() agentrunner.WatchFunc {
	return func(e agentrunner.RunnerEvent) {
		if f.hub == nil {
			return
		}
		// ChatID is empty on a `displaced` frame and must stay empty: it is what tells
		// a client to stop showing this runner as the chat's agent.
		f.hub.BroadcastAgentRunner(e.RunnerID, e.WorkspaceID, e.ChatID, e.Kind)
	}
}
