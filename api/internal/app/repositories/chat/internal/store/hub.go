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

// ChatEvent is one projected agentchat lifecycle event, in repo-owned terms.
// It carries the bare (chatID, workspaceID, kind, working) facts a lifecycle
// frame is built from. This is the
// wire shape the FE consumes as dto.AgentChatEvent (00 agentic-engine spec §7) — a
// bare id+kind frame plus the owning workspace, not the full aggregate — so a
// Hub.BroadcastAgentChat-shaped func wires straight through with no adapter.
//
// working is the aggregate's OWN folded answer (domain.Chat.Working) as of this
// event, and it rides along because the alternative is a second implementation of the
// fold in TypeScript — which is not a hypothetical cost. The FE used to re-derive the
// spinner from the KIND alone (turn_stopped → idle), so the instant "the turn ended"
// stopped meaning "the agent is done", the two answers disagreed and the chat row went
// dark while the aggregate said Working. One fold, computed once, read everywhere.
//
// This is NOT the snapshot the spec refuses: it is one derived fact ABOUT the event
// being announced, not the chat's state for the client to keep in lieu of reading it.
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

type ChatEvent struct {
	ChatID      string
	WorkspaceID string
	Kind        string
	Working     bool
	Forgotten   bool
}

// WatchFunc receives every projected agentchat event. It replaces the former
// BroadcastFunc: the repository announces WHAT HAPPENED, and the usecase decides what
// the frontend is told (usecases/agent/internal/fanout). Registering the subscription
// is the repository's job only because asynx subscription is; shaping a wire frame is
// not.
type WatchFunc func(ChatEvent)

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
	ax asynx.Asynx[domain.Chat],
	watch WatchFunc,
) error {
	p := &hubProjector{watch: watch}
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
	if _, err := ax.OnForget(func(_ context.Context, evt asynxModels.Event[domain.Chat]) {
		p.onForget(evt)
	}); err != nil {
		return fmt.Errorf("agentchat hub projection: onforget: %w", err)
	}
	return nil
}

type hubProjector struct {
	watch WatchFunc
}

// emit is the single exit. A nil watch degrades to a no-op so a store built without
// one (every unit test) never panics.
func (p *hubProjector) emit(e ChatEvent) {
	if p.watch == nil {
		return
	}
	p.watch(e)
}

// onForget announces a hard delete. A forgotten chat is not working: it is not
// anything. The client drops it.
func (p *hubProjector) onForget(evt asynxModels.Event[domain.Chat]) {
	p.emit(ChatEvent{
		ChatID:      evt.Aggregate.ID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Kind:        "deleted",
		Forgotten:   true,
	})
}

// onEvent broadcasts the (chatID, workspaceID, kind, working) lifecycle frame
// derived from the event. workspaceID and working both come off the REDUCED
// aggregate (evt.Aggregate), not the event name/id: neither is encoded in
// EventName, and Working in particular is the fold's own output as of this event —
// exactly the value the FE must show, computed by the one authority for it. It
// never persists — the store projection owns durability.
func (p *hubProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) {
	_ = ctx
	p.emit(ChatEvent{
		ChatID:      evt.AggregateID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Kind:        eventKind(evt.EventName),
		Working:     evt.Aggregate.Working,
	})
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
