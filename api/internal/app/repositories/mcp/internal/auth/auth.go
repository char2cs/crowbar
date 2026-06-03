// Package auth resolves an X-Agent-Run-Token header value into a full
// AgentContext (AgentRun, Task, and the resolved StateDefinition from the
// flow). All MCP tool handlers call Resolve before executing any business
// logic.
package auth

import (
	"context"
	"errors"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	mcpinternal "github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// ErrUnauthorized is returned when the token is missing, not found, or the
// run is not in the running state.
var ErrUnauthorized = errors.New("unauthorized")

// ErrForbidden is returned when the authenticated agent attempts to use a
// tool that is not declared in its current state.
var ErrForbidden = errors.New("forbidden")

// AgentContext is the resolved identity of a calling agent: the current run,
// its parent task, and the flow state the task is currently in.
type AgentContext struct {
	AgentRun domain.AgentRun
	Task     domain.Task
	State    flow.StateDefinition
	Flow     flow.FlowDefinition
}

// Resolve validates token and builds the full AgentContext.
//
// Steps:
//  1. Look up AgentRun by token; return ErrUnauthorized if not found.
//  2. Verify AgentRun.Status == running; stale tokens are rejected.
//  3. Load the parent Task.
//  4. Load the FlowDefinition for the task's flow path.
//  5. Find the StateDefinition matching task.CurrentState.
//  6. Return AgentContext.
func Resolve(
	ctx context.Context,
	token string,
	agentRunRepo mcpinternal.AgentRunReader,
	taskRepo mcpinternal.TaskReader,
	flowLoader flow.Loader,
) (AgentContext, error) {
	if token == "" {
		return AgentContext{}, ErrUnauthorized
	}

	run, err := agentRunRepo.GetByToken(ctx, token)
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return AgentContext{}, ErrUnauthorized
		}
		return AgentContext{}, fmt.Errorf("auth: get by token: %w", err)
	}

	if run.Status != domain.AgentRunStatusRunning {
		return AgentContext{}, ErrUnauthorized
	}

	task, err := taskRepo.Get(ctx, run.TaskID)
	if err != nil {
		return AgentContext{}, fmt.Errorf("auth: get task %s: %w", run.TaskID, err)
	}

	flowDef, err := flowLoader.Load(task.FlowPath)
	if err != nil {
		return AgentContext{}, fmt.Errorf("auth: load flow %q: %w", task.FlowPath, err)
	}

	var stateDef flow.StateDefinition
	found := false
	for _, s := range flowDef.States {
		if s.Name == task.CurrentState {
			stateDef = s
			found = true
			break
		}
	}
	if !found {
		return AgentContext{}, fmt.Errorf("auth: state %q not found in flow", task.CurrentState)
	}

	return AgentContext{
		AgentRun: run,
		Task:     task,
		State:    stateDef,
		Flow:     flowDef,
	}, nil
}

// HasTool reports whether toolName is declared in the agent context's current
// state tool list.
func HasTool(actx AgentContext, toolName string) bool {
	if actx.State.Agent == nil {
		return false
	}
	for _, t := range actx.State.Agent.Tools {
		if t == toolName {
			return true
		}
	}
	return false
}
