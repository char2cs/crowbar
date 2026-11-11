package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatEvent is one projected agentchat lifecycle event, in repo-owned terms:
// the chat as of it and its workspace, or a Forget. It is the single source of
// agent-chat row frames.
type ChatEvent struct {
	ChatID      string
	WorkspaceID string
	Forgotten   bool
	// Chat is the aggregate as of this event (the command-side fold, never the
	// lagging read model) and Version its version, so a frame never describes a
	// chat older than the event it announces.
	Chat    domain.Chat
	Version int64
}

// WatchFunc receives every projected agentchat event. The repository announces
// what happened; the usecase decides what the frontend is told.
type WatchFunc func(ChatEvent)

// registerHubProjection hands every agentchat event and every Forget to watch.
// It never touches the read model, so it and the store projection derive
// independently from the same stream. Register it once.
func registerHubProjection(
	ax asynx.Asynx[domain.Chat],
	watch WatchFunc,
) error {
	p := &hubProjector{watch: watch}
	if _, err := ax.Subscribe(asynx.Topic("agentchat.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentchat hub projection: subscribe: %w", err)
	}
	// ax.Forget fires only OnForget, not the "agentchat.*" topic, so without
	// this no live client would learn the chat is gone. evt.Aggregate carries
	// the last-known state, WorkspaceID included.
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
		Forgotten:   true,
		Chat:        evt.Aggregate,
		Version:     evt.Version,
	})
}

// onEvent broadcasts the lifecycle frame derived from the event. workspaceID
// comes off the reduced aggregate: EventName does not encode it.
func (p *hubProjector) onEvent(
	_ context.Context,
	evt asynxModels.Event[domain.Chat],
) {
	p.emit(ChatEvent{
		ChatID:      evt.AggregateID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Chat:        evt.Aggregate,
		Version:     evt.Version,
	})
}
