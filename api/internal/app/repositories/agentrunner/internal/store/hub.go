package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventNamePrefix is the aggregate-name segment every agentrunner command's
// EventName() puts first ("agentrunner.<kind>.<id>" — see internal/commands/*.go).
// eventKind strips it to isolate <kind>.
const eventNamePrefix = "agentrunner."

// BroadcastFunc receives every projected AgentRunner event as a bare
// (runnerID, workspaceID, chatID, kind) lifecycle frame for hub fan-out.
// chatID is the chat the runner is pointed at AS OF this event, so a `moved`
// frame names the chat it entered — the frame carries placement, never liveness.
//
// kind is the <kind> segment of the emitting command's EventName, so the
// vocabulary is exactly one term per command:
//
//	started       — a CLI was spawned into a chat
//	session_bound — the provider announced its first conversation id
//	moved         — the CLI repointed at another chat/conversation (/clear, /resume)
//	displaced     — Crowbar took the CLI OFF its chat (an eviction, the outgoing side of a
//	                switch, a chat deleted under it). chatID is EMPTY on this frame: the
//	                runner is on no chat. The process is still alive — this says nothing
//	                about liveness — but it no longer holds anything, so a client following
//	                that runner should stop showing it as the chat's agent WITHOUT waiting
//	                for the exit, which may be a second away or (if the kill failed) never.
//	exited        — the PTY died; the live row is gone and the chat is now dormant
type BroadcastFunc func(
	runnerID string,
	workspaceID string,
	chatID string,
	kind string,
)

// registerHubProjection subscribes the WS fan-out projection to every agentrunner
// event: it derives the lifecycle kind from evt.EventName and hands the frame to
// broadcast. It never persists — the store projection (store.go) owns durability,
// so the two derive independently from the same event stream and cannot drift.
// Designed to register ONCE, on the singleton ax.
func registerHubProjection(
	ax asynx.Asynx[domain.AgentRunner],
	broadcast BroadcastFunc,
) error {
	p := &hubProjector{broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("agentrunner.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentrunner hub projection: subscribe: %w", err)
	}
	return nil
}

type hubProjector struct {
	broadcast BroadcastFunc
}

func (p *hubProjector) onEvent(
	_ context.Context,
	evt asynxModels.Event[domain.AgentRunner],
) {
	r := evt.Aggregate
	p.broadcast(evt.AggregateID, r.WorkspaceID, r.CurrentChatID, eventKind(evt.EventName))
}

// eventKind extracts the <kind> segment from an agentrunner EventName
// ("agentrunner.<kind>.<id>"). Falls back to the name with only the aggregate
// prefix stripped if a future command ever emits a name that doesn't fit the
// pattern, rather than silently dropping the frame.
func eventKind(
	eventName string,
) string {
	rest := strings.TrimPrefix(eventName, eventNamePrefix)
	kind, _, found := strings.Cut(rest, ".")
	if !found {
		return rest
	}
	return kind
}
