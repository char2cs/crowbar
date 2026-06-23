package chat

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/internal/store"
	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
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
	ax      asynx.Asynx[domain.Chat]
	store   store.Store
	writeMu serialize.KeyedMutex
}

// New builds a Chat repository over the asynx instance and a GORM DB. The
// broadcast func is the hub fan-out for projected rows (01 §5).
//
// The chat aggregate is global: one asynx and one event store hold every chat. es
// is that shared event store; when it exposes serialize.AggregateLister, New
// enumerates its ids so store.New can reconcile each read-model row from the event
// log on open (heals rows a dropped projection left missing). Enumeration is
// best-effort — a failure logs and falls back to no reconcile, exactly as if no
// row needed healing.
func New(
	ctx context.Context,
	ax asynx.Asynx[domain.Chat],
	es asynxModels.Store,
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (Chat, error) {
	ids := aggregateIDs(ctx, es)
	st, err := store.New(ctx, db, ax, broadcast, ids)
	if err != nil {
		return nil, fmt.Errorf("chat: store: %w", err)
	}
	return &chat{ax: ax, store: st}, nil
}

// aggregateIDs enumerates the event store's aggregate ids when it supports the
// optional AggregateLister capability, so the read model can reconcile every chat
// on open. A store without the capability, or an enumeration error, yields no ids
// (reconcile is then a no-op).
func aggregateIDs(
	ctx context.Context,
	es asynxModels.Store,
) []string {
	lister, ok := es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	ids, err := lister.AggregateIDs(ctx)
	if err != nil {
		slog.WarnContext(ctx, "chat: enumerate aggregate ids for reconcile failed; skipping reconcile", "err", err)
		return nil
	}
	return ids
}

func (c *chat) Create(
	ctx context.Context,
	id string,
	wsID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	c.writeMu.Lock(id)
	defer c.writeMu.Unlock(id)
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
	c.writeMu.Lock(id)
	defer c.writeMu.Unlock(id)
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
	c.writeMu.Lock(id)
	defer c.writeMu.Unlock(id)
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
	c.writeMu.Lock(id)
	defer c.writeMu.Unlock(id)
	evt, err := c.ax.SendWait(ctx, commands.DeleteChat{ID: id, Now: now})
	if err != nil {
		return domain.Chat{}, fmt.Errorf("chat: delete: %w", err)
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
