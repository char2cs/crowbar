package chat

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc is an alias for the store-layer broadcast type, exposed so the
// repositories container can wire it without importing the internal store package.
type BroadcastFunc = store.BroadcastFunc

// Chat is the chat aggregate repository.
type Chat interface {
	Create(
		ctx context.Context,
		id string,
		wsID string,
		title string,
		now time.Time,
	) (domain.Chat, error)
	Fork(
		ctx context.Context,
		id string,
		wsID string,
		parentID string,
		title string,
		now time.Time,
	) (domain.Chat, error)
	Rename(
		ctx context.Context,
		id string,
		title string,
	) (domain.Chat, error)
	Delete(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Chat, error)
	ResetIdle(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	SetAgentRunning(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.Chat, error)
	List(
		ctx context.Context,
	) ([]domain.Chat, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
}

type chat struct {
	ax    asynx.Asynx[domain.Chat]
	store store.Store
}

// New builds a Chat repository over the asynx instance and a GORM DB. The
// broadcast func is the hub fan-out for projected rows (01 §5).
func New(
	ax asynx.Asynx[domain.Chat],
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (Chat, error) {
	st, err := store.New(db, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("chat: store: %w", err)
	}
	return &chat{ax: ax, store: st}, nil
}

func (c *chat) Create(
	ctx context.Context,
	id string,
	wsID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.CreateChat{ID: id, WsID: wsID, Title: title, Now: now})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) Fork(
	ctx context.Context,
	id string,
	wsID string,
	parentID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.ForkChat{
		ID:       id,
		WsID:     wsID,
		ParentID: parentID,
		Title:    title,
		Now:      now,
	})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: fork: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) Rename(
	ctx context.Context,
	id string,
	title string,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.RenameChat{ID: id, Title: title})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: rename: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) Delete(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.DeleteChat{ID: id, Now: now})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: delete: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) ResetIdle(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.ResetChatIdle{ID: id})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: reset idle: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) SetAgentRunning(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	evt, err := c.ax.SendWait(ctx, commands.SetChatAgentRunning{ID: id})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: set agent running: %w", err)
	}
	return evt.Aggregate, nil
}

func (c *chat) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	got, err := c.ax.Get(ctx, id)
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: get: %w", err)
	}
	return got, nil
}

func (c *chat) List(
	ctx context.Context,
) ([]domain.Chat, error) {
	rows, err := c.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("chat: list: %w", err)
	}
	return rows, nil
}

func (c *chat) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	rows, err := c.store.ListByWorkspace(ctx, wsID)
	if err != nil {
		return nil, fmt.Errorf("chat: list by workspace: %w", err)
	}
	return rows, nil
}
