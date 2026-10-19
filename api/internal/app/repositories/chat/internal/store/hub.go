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

// ChatEvent is one projected agentchat lifecycle event, in repo-owned terms:
// the chat, its workspace, and kind, the <kind> segment of the emitting
// command's EventName ("agentchat.<kind>.<id>"), or "deleted" for a Forget.
// It is the single source of agent-chat row frames.
type ChatEvent struct {
	ChatID      string
	WorkspaceID string
	Kind        string
	Forgotten   bool
	// Chat is the reduced aggregate AS OF this event — the command-side fold,
	// never the lagging read model — and Version its aggregate version. They
	// are what the chat snapshot owner (usecases/chat/internal/snapshot) is fed
	// from, so a frame never describes a chat older than the event it announces.
	Chat    domain.Chat
	Version int64
}

// WatchFunc receives every projected agentchat event. It replaces the former
// BroadcastFunc: the repository announces WHAT HAPPENED, and the usecase decides what
// the frontend is told (usecases/chat/internal/fanout). Registering the subscription
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

// onForget announces a hard delete. The client drops the chat.
func (p *hubProjector) onForget(evt asynxModels.Event[domain.Chat]) {
	p.emit(ChatEvent{
		ChatID:      evt.Aggregate.ID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Kind:        "deleted",
		Forgotten:   true,
		Chat:        evt.Aggregate,
		Version:     evt.Version,
	})
}

// onEvent broadcasts the lifecycle frame derived from the event. workspaceID
// comes off the reduced aggregate: EventName does not encode it.
func (p *hubProjector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.Chat],
) {
	_ = ctx
	p.emit(ChatEvent{
		ChatID:      evt.AggregateID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Kind:        eventKind(evt.EventName),
		Chat:        evt.Aggregate,
		Version:     evt.Version,
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
