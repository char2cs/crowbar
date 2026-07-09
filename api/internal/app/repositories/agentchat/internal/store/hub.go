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
// (chatID, kind) lifecycle frame for hub fan-out. This is the SAME wire shape
// the usecase's Broadcaster.BroadcastAgentChat(chatID, kind string) already
// produces today (api/internal/app/usecases/agent/agent.go) and the FE already
// consumes as dto.AgentChatEvent (00 agentic-engine spec §7) — a bare id+kind
// frame, not the full aggregate — so the T10 cutover can wire a
// Hub.BroadcastAgentChat-shaped func straight through with no adapter.
type BroadcastFunc func(
	chatID string,
	kind string,
)

// registerHubProjection subscribes the hub (WS fan-out) projection to every
// agentchat event on the singleton axAgentChat: for each event it derives the
// lifecycle kind from evt.EventName ("agentchat.<kind>.<id>", set by
// internal/commands/*.go) and hands (evt.AggregateID, kind) to broadcast. It
// will replace the usecase's manual BroadcastAgentChat calls at the T10
// cutover; until then this registration is additive — it does NOT touch the
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
