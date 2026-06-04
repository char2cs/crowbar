package agentrun

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/asynx"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
}

type agentRun struct {
	ax asynx.Asynx[domain.AgentRun]
}

// New builds an AgentRun repository over the given asynx instance.
func New(
	ax asynx.Asynx[domain.AgentRun],
) AgentRun {
	return &agentRun{ax: ax}
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
