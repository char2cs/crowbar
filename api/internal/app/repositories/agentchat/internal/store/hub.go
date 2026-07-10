package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventNamePrefix is the aggregate-name segment every agentchat command's
// EventName() puts first ("agentchat.<kind>.<id>" — see
// internal/commands/*.go). eventKind strips it to isolate <kind>.
const eventNamePrefix = "agentchat."

// BroadcastFunc receives every projected AgentChat event as a bare
// (chatID, workspaceID, kind) lifecycle frame for hub fan-out. This is the wire
// shape the FE consumes as dto.AgentChatEvent (00 agentic-engine spec §7) — a
// bare id+kind frame plus the owning workspace, not the full aggregate — so a
// Hub.BroadcastAgentChat-shaped func wires straight through with no adapter.
// workspaceID is carried on every frame and now scopes the WS fan-out per
// workspace (Task 3): the agent-chat StreamDef filters frames by WorkspaceID
// against the subscribed :wsId, so a client only sees its own workspace's chats.
//
// kind is the <kind> segment of the emitting command's EventName
// ("agentchat.<kind>.<id>"), so the full lifecycle vocabulary is exactly one
// term per command:
//
//	created         — a fresh chat + its first segment (SpawnChat / a moved-to new chat)
//	segment_opened  — a new active segment on an existing chat (switch-in / resume / focus)
//	segment_ended   — the active segment closed (switch-out / process exit / boot reconcile)
//	session_bound   — a provider's native session id recorded on a segment
//	turn_started    — a turn opened (user_prompt hook → Working)
//	turn_stopped    — a turn closed (turn_stop hook / boot reconcile → idle)
//	title_set       — the chat title changed (derived / agent / user rename)
//	deleted         — the chat was hard-deleted (asynx Forget, see OnForget below;
//	                  NOT emitted via a command's EventName like the other kinds)
//
// This is the SINGLE source of agent-chat frames: the usecase no longer
// broadcasts, so a lifecycle change is exactly one event → exactly one frame.

type BroadcastFunc func(
	chatID string,
	workspaceID string,
	kind string,
)

// registerHubProjection subscribes the hub (WS fan-out) projection to every
// agentchat event on the singleton axAgentChat: for each event it derives the
// lifecycle kind from evt.EventName ("agentchat.<kind>.<id>", set by
// internal/commands/*.go) and hands (evt.AggregateID, kind) to broadcast. It is
// the sole broadcaster of agent-chat lifecycle frames — the usecase's manual
// BroadcastAgentChat calls were retired at cutover. It does NOT touch the
// durable read model, so it and the store projection (store.go) derive
// independently from the same event stream and cannot drift. Designed to
// register ONCE on the singleton.
func registerHubProjection(
	ax asynx.Asynx[domain.AgentChat],
	broadcast BroadcastFunc,
) error {
	p := &hubProjector{broadcast: broadcast}
	if _, err := ax.Subscribe(asynx.Topic("agentchat.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentchat hub projection: subscribe: %w", err)
	}
	// ax.Forget (the hard-delete path, Task 5) fires ONLY the
	// "asynx.aggregate.forget" topic via OnForget — it is not one of the
	// commands Subscribe's "agentchat.*" pattern matches — so a Forget would
	// otherwise never reach the hub and no live client would ever learn the
	// chat is gone. evt.Aggregate carries the aggregate's last-known state
	// (including WorkspaceID) at the moment it was forgotten, exactly like
	// every other projected event.
	if _, err := ax.OnForget(func(_ context.Context, evt asynxModels.Event[domain.AgentChat]) {
		p.broadcast(evt.Aggregate.ID, evt.Aggregate.WorkspaceID, "deleted")
	}); err != nil {
		return fmt.Errorf("agentchat hub projection: onforget: %w", err)
	}
	return nil
}

type hubProjector struct {
	broadcast BroadcastFunc
}

// onEvent broadcasts the (chatID, workspaceID, kind) lifecycle frame derived
// from the event. workspaceID comes off the reduced aggregate
// (evt.Aggregate.WorkspaceID), not the event name/id, since it is not encoded
// in EventName. It never persists — the store projection owns durability.
func (p *hubProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentChat],
) {
	_ = ctx
	p.broadcast(evt.AggregateID, evt.Aggregate.WorkspaceID, eventKind(evt.EventName))
}

// eventKind extracts the <kind> segment from an agentchat EventName
// ("agentchat.<kind>.<id>"). Falls back to the name with only the aggregate
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
