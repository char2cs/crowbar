package agentrun

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/commands"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/store"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// BroadcastFunc is an alias for the store-layer broadcast type, exposed so the
// repositories container can wire it without importing the internal store package.
type BroadcastFunc = store.BroadcastFunc

// AgentRun is the agent-run aggregate repository.
type AgentRun interface {
	Create(
		ctx context.Context,
		id string,
		wsID string,
		chatID string,
		now time.Time,
	) (domain.AgentRun, error)
	MarkRunning(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Complete(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Fail(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.AgentRun, error)
	List(
		ctx context.Context,
	) ([]domain.AgentRun, error)
	ListRunning(
		ctx context.Context,
	) ([]domain.AgentRun, error)
	ListByChat(
		ctx context.Context,
		chatID string,
	) ([]domain.AgentRun, error)
}

type agentRun struct {
	ax    asynx.Asynx[domain.AgentRun]
	store store.Store
}

// New builds an AgentRun repository over the given asynx instance and a GORM DB.
// The broadcast func is the hub fan-out for projected rows (03 §2).
func New(
	ax asynx.Asynx[domain.AgentRun],
	db *gormdb.DB,
	broadcast store.BroadcastFunc,
) (AgentRun, error) {
	st, err := store.New(db, ax, broadcast)
	if err != nil {
		return nil, fmt.Errorf("agentrun: store: %w", err)
	}
	return &agentRun{ax: ax, store: st}, nil
}

func (r *agentRun) Create(
	ctx context.Context,
	id string,
	wsID string,
	chatID string,
	now time.Time,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.CreateAgentRun{ID: id, WsID: wsID, ChatID: chatID, Now: now})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: create: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) MarkRunning(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.MarkAgentRunRunning{ID: id})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: mark running: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) Complete(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.CompleteAgentRun{ID: id})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: complete: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) Fail(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	evt, err := r.ax.SendWait(ctx, commands.FailAgentRun{ID: id})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: fail: %w", err)
	}
	return evt.Aggregate, nil
}

func (r *agentRun) Get(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	got, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: get: %w", err)
	}
	return got, nil
}

func (r *agentRun) List(
	ctx context.Context,
) ([]domain.AgentRun, error) {
	rows, err := r.store.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentrun: list: %w", err)
	}
	return rows, nil
}

func (r *agentRun) ListRunning(
	ctx context.Context,
) ([]domain.AgentRun, error) {
	rows, err := r.store.ListRunning(ctx)
	if err != nil {
		return nil, fmt.Errorf("agentrun: list running: %w", err)
	}
	return rows, nil
}

func (r *agentRun) ListByChat(
	ctx context.Context,
	chatID string,
) ([]domain.AgentRun, error) {
	rows, err := r.store.ListByChat(ctx, chatID)
	if err != nil {
		return nil, fmt.Errorf("agentrun: list by chat: %w", err)
	}
	return rows, nil
}
