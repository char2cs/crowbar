package dispatcher

import (
	"context"
	"fmt"
	"log/slog"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/app/repositories/task"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// Dispatcher subscribes to all task events and dispatches AgentRuntime.Run
// whenever a task enters a flow state that has an Agent configured.
type Dispatcher struct {
	ctx          context.Context
	taskRepo     task.Task
	agentRunRepo agentrun.AgentRun
	flowLoader   flow.Loader
	runtime      agent.AgentRuntime
}

// New constructs a Dispatcher. If runtime is nil, Register still succeeds but
// all dispatch calls are silently skipped — useful when agent execution is
// intentionally disabled (e.g. tests that only exercise the REST layer).
func New(
	ctx context.Context,
	taskRepo task.Task,
	agentRunRepo agentrun.AgentRun,
	flowLoader flow.Loader,
	runtime agent.AgentRuntime,
) *Dispatcher {
	return &Dispatcher{
		ctx:          ctx,
		taskRepo:     taskRepo,
		agentRunRepo: agentRunRepo,
		flowLoader:   flowLoader,
		runtime:      runtime,
	}
}

// Register subscribes to all task events. Call once after all repositories are ready.
func (d *Dispatcher) Register() error {
	if _, err := d.taskRepo.Subscribe("task.*", d.handleEvent); err != nil {
		return fmt.Errorf("dispatcher: subscribe: %w", err)
	}
	return nil
}

func (d *Dispatcher) handleEvent(
	_ context.Context,
	evt asynxModels.Event[domain.Task],
) {
	if d.runtime == nil {
		return
	}

	t := evt.Aggregate

	if t.Status == domain.TaskStatusArchived || t.Status == domain.TaskStatusPaused {
		return
	}

	flowDef, err := d.flowLoader.Load(t.FlowPath)
	if err != nil {
		slog.WarnContext(d.ctx, "dispatcher: load flow", "task_id", t.ID, "err", err)
		return
	}

	var stateDef *flow.StateDefinition
	for i := range flowDef.States {
		if flowDef.States[i].Name == t.CurrentState {
			stateDef = &flowDef.States[i]
			break
		}
	}

	if stateDef == nil || stateDef.Agent == nil {
		return
	}

	token := uuid.NewString()
	run, err := d.agentRunRepo.Create(d.ctx, t.ID, t.CurrentState, token)
	if err != nil {
		slog.WarnContext(d.ctx, "dispatcher: create agent run", "task_id", t.ID, "err", err)
		return
	}

	go func() {
		if err := d.runtime.Run(d.ctx, run, t, *stateDef); err != nil {
			slog.WarnContext(d.ctx, "dispatcher: agent run failed", "run_id", run.ID, "err", err)
			if failErr := d.agentRunRepo.FailAgentRun(d.ctx, run.ID); failErr != nil {
				slog.WarnContext(d.ctx, "dispatcher: fail agent run", "run_id", run.ID, "err", failErr)
			}
		}
	}()
}
