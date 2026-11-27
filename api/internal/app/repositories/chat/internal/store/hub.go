package store

import (
	"context"
	"fmt"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type ChatEvent struct {
	ChatID      string
	WorkspaceID string
	Forgotten   bool
	Chat        domain.Chat
	Version     int64
}

type WatchFunc func(ChatEvent)

func registerHubProjection(
	ax asynx.Asynx[domain.Chat],
	watch WatchFunc,
) error {
	p := &hubProjector{watch: watch}
	if _, err := ax.Subscribe(asynx.Topic("agentchat.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentchat hub projection: subscribe: %w", err)
	}
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

func (p *hubProjector) emit(e ChatEvent) {
	if p.watch == nil {
		return
	}
	p.watch(e)
}

func (p *hubProjector) onForget(evt asynxModels.Event[domain.Chat]) {
	p.emit(ChatEvent{
		ChatID:      evt.Aggregate.ID,
		WorkspaceID: evt.Aggregate.WorkspaceID,
		Forgotten:   true,
		Chat:        evt.Aggregate,
		Version:     evt.Version,
	})
}

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
