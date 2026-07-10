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
// (chatID, kind) lifecycle frame for hub fan-out. This is the wire shape the FE
// consumes as dto.AgentChatEvent (00 agentic-engine spec §7) — a bare id+kind
// frame, not the full aggregate — so a Hub.BroadcastAgentChat-shaped func wires
// straight through with no adapter.
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
//	deleted         — the chat was soft-deleted (tombstoned)
//
// This is the SINGLE source of agent-chat frames: the usecase no longer
// broadcasts, so a lifecycle change is exactly one event → exactly one frame.

type BroadcastFunc func(
	chatID string,
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
	return nil
}

type hubProjector struct {
	broadcast BroadcastFunc
}

// onEvent broadcasts the (chatID, kind) lifecycle frame derived from the
// event. It never persists — the store projection owns durability.
func (p *hubProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentChat],
) {
	_ = ctx
	p.broadcast(evt.AggregateID, eventKind(evt.EventName))
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
