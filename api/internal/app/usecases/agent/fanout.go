package agent

import (
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/fanout"
)

// Fanout shapes repository lifecycle announcements into frontend frames.
//
// It is built by the composition root rather than by agent.New because the agent
// repositories are constructed BEFORE the usecase that reads them, and they need
// their watch seams at construction. The decision of what a client is told still
// lives here, in the usecase layer, which is the whole point of the seam.
type Fanout = fanout.Fanout

// Hub is the WS broadcaster the fanout needs. *hub.Hub satisfies it.
type Hub = fanout.Hub

// NewFanout builds the fanout over hub. A nil hub degrades to a no-op.
func NewFanout(hub Hub) *Fanout { return fanout.New(hub) }

var (
	_ = agentchat.WatchFunc(nil)
	_ = agentrunner.WatchFunc(nil)
)
