package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type CreateInput struct {
	ID          string
	WorkspaceID string
	RepoID      string
	Type        domain.ChatType
	Surface     string
	ProviderID  string
	Now         time.Time
}

type EventStore interface {
	Create(
		ctx context.Context,
		in CreateInput,
	) (domain.Chat, error)
	SetTitle(
		ctx context.Context,
		chatID string,
		title string,
		source string,
	) (domain.Chat, error)
	SetSelection(
		ctx context.Context,
		chatID string,
		model string,
		effort string,
	) (domain.Chat, error)
	SetPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) (domain.Chat, error)
	SetSurface(
		ctx context.Context,
		chatID string,
		surface string,
	) (domain.Chat, error)
	SetProvider(
		ctx context.Context,
		chatID string,
		providerID string,
	) (domain.Chat, error)
	SeedPermissionLevel(
		ctx context.Context,
		chatID string,
		level string,
	) (domain.Chat, error)
	LoadChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	SetPlacement(
		ctx context.Context,
		chatID string,
		parentID string,
		order int,
	) (domain.Chat, error)
	SetOrder(
		ctx context.Context,
		chatID string,
		order int,
	) (domain.Chat, error)
	SetWorkspace(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) (domain.Chat, error)
	RunIn(
		ctx context.Context,
		chatID string,
		workspaceID string,
	) (domain.Chat, error)
	SetType(
		ctx context.Context,
		chatID string,
		chatType domain.ChatType,
	) (domain.Chat, error)
	Forget(
		ctx context.Context,
		id string,
	) error
	GetChat(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	GetChats(
		ctx context.Context,
		ids []string,
	) ([]domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	ListChats(
		ctx context.Context,
	) ([]domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
}

type eventSourced struct {
	ax         asynx.Asynx[domain.Chat]
	store      store.Store
	placements Placements
}

func NewEventSourced(
	ctx context.Context,
	ax asynx.Asynx[domain.Chat],
	es asynxModels.Store,
	rm *ReadModel,
	watch WatchFunc,
	placements Placements,
) (EventStore, error) {
	r := &eventSourced{ax: ax, placements: placements}
	st, err := store.New(rm, es, ax, r.placedWatch(ctx, watch))
	if err != nil {
		return nil, fmt.Errorf("agentchat: store: %w", err)
	}
	r.store = st
	return r, nil
}

func (r *eventSourced) Create(
	ctx context.Context,
	in CreateInput,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.Create{
		ID:          in.ID,
		WorkspaceID: in.WorkspaceID,
		RepoID:      in.RepoID,
		Type:        in.Type,
		Surface:     in.Surface,
		ProviderID:  in.ProviderID,
		Now:         in.Now,
	})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: create: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetTitle(
	ctx context.Context,
	chatID string,
	title string,
	source string,
) (domain.Chat, error) {
	evt, err := r.ax.Send(ctx, commands.SetTitle{ChatID: chatID, Title: title, Source: source})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set title: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetSelection(
	ctx context.Context,
	chatID string,
	model string,
	effort string,
) (domain.Chat, error) {
	evt, err := r.ax.Send(ctx, commands.SetSelection{ChatID: chatID, Model: model, Effort: effort})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set selection: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetPermissionLevel{ChatID: chatID, Level: level, Explicit: true})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set permission level: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetSurface(
	ctx context.Context,
	chatID string,
	surface string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetSurface{ChatID: chatID, Surface: surface})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set surface: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetProvider(
	ctx context.Context,
	chatID string,
	providerID string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetProvider{ChatID: chatID, ProviderID: providerID})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set provider: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SeedPermissionLevel(
	ctx context.Context,
	chatID string,
	level string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetPermissionLevel{ChatID: chatID, Level: level, Explicit: false})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: seed permission level: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) LoadChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	chat, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: load chat: %w", mapNotFound(err))
	}
	return r.placed(ctx, chat)
}

func (r *eventSourced) SetWorkspace(
	ctx context.Context,
	chatID string,
	workspaceID string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetWorkspace{ID: chatID, WorkspaceID: workspaceID})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set workspace: %w", err)
	}
	if err := r.placements.AdoptChat(ctx, chatID, workspaceID); err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set workspace: placement: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) RunIn(
	ctx context.Context,
	chatID string,
	workspaceID string,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetWorkspace{ID: chatID, WorkspaceID: workspaceID, Shares: true})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: run in: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) SetType(
	ctx context.Context,
	chatID string,
	chatType domain.ChatType,
) (domain.Chat, error) {
	evt, err := r.ax.SendWait(ctx, commands.SetType{ID: chatID, Type: chatType})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: set type: %w", err)
	}
	return r.placed(ctx, evt.Aggregate)
}

func (r *eventSourced) Forget(
	ctx context.Context,
	id string,
) error {
	if err := r.ax.Forget(ctx, id); err != nil {
		return fmt.Errorf("agentchat: forget: %w", err)
	}
	if err := r.placements.DropChat(ctx, id); err != nil {
		return fmt.Errorf("agentchat: forget: placement: %w", err)
	}
	return nil
}

func (r *eventSourced) GetChat(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	chat, err := r.store.GetChat(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("agentchat: get chat: %w", mapNotFound(err))
	}
	return r.placed(ctx, chat)
}

func (r *eventSourced) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return r.GetChat(ctx, id)
}

func (r *eventSourced) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := r.store.ListChats(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentchat: list chats: %w", err)
	}
	return r.placedAll(ctx, rows)
}

func (r *eventSourced) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	rows, err := r.store.ListByWorkspace(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("agentchat: list by workspace: %w", err)
	}
	return r.placedAll(ctx, rows)
}

func mapNotFound(
	err error,
) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, asynxModels.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
