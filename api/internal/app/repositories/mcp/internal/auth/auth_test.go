package auth_test

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"

	"github.com/char2cs/crowbar/api/internal/app/repositories/mcp/internal/auth"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// --- minimal mock implementations ---

type mockAgentRunReader struct {
	run domain.AgentRun
	err error
}

func (m *mockAgentRunReader) GetByToken(_ context.Context, _ string) (domain.AgentRun, error) {
	return m.run, m.err
}
func (m *mockAgentRunReader) CompleteAgentRun(_ context.Context, _ string, _ string) error {
	return nil
}

type mockTaskReader struct {
	task domain.Task
	err  error
}

func (m *mockTaskReader) Get(_ context.Context, _ string) (domain.Task, error) {
	return m.task, m.err
}
func (m *mockTaskReader) AdvanceState(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

type mockFlowLoader struct {
	def flow.FlowDefinition
	err error
}

func (m *mockFlowLoader) Load(_ string) (flow.FlowDefinition, error) {
	return m.def, m.err
}

// --- helper for a minimal valid flow ---

func validFlow(stateName string, tools []string) flow.FlowDefinition {
	return flow.FlowDefinition{
		Name: "test-flow",
		States: []flow.StateDefinition{
			{
				Name:  stateName,
				Items: false,
				Agent: &flow.AgentDef{Tools: tools},
				Transitions: []flow.TransitionDef{
					{On: "done", To: "next_state"},
				},
			},
		},
	}
}

// --- Resolve tests ---

func TestResolve_EmptyToken(t *testing.T) {
	_, err := auth.Resolve(
		context.Background(),
		"",
		&mockAgentRunReader{},
		&mockTaskReader{},
		&mockFlowLoader{},
	)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestResolve_TokenNotFound(t *testing.T) {
	_, err := auth.Resolve(
		context.Background(),
		"bad-token",
		&mockAgentRunReader{err: asynxModels.ErrNotFound},
		&mockTaskReader{},
		&mockFlowLoader{},
	)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestResolve_GetByTokenOtherError(t *testing.T) {
	someErr := errors.New("db down")
	_, err := auth.Resolve(
		context.Background(),
		"tok",
		&mockAgentRunReader{err: someErr},
		&mockTaskReader{},
		&mockFlowLoader{},
	)
	if errors.Is(err, auth.ErrUnauthorized) {
		t.Fatal("should not return ErrUnauthorized for arbitrary DB error")
	}
	if !errors.Is(err, someErr) {
		t.Fatalf("expected wrapped someErr, got %v", err)
	}
}

func TestResolve_RunNotRunning(t *testing.T) {
	run := domain.AgentRun{
		ID:     "r1",
		TaskID: "t1",
		Status: domain.AgentRunStatusCompleted,
	}
	_, err := auth.Resolve(
		context.Background(),
		"tok",
		&mockAgentRunReader{run: run},
		&mockTaskReader{},
		&mockFlowLoader{},
	)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for non-running run, got %v", err)
	}
}

func TestResolve_RunNotRunning_Failed(t *testing.T) {
	run := domain.AgentRun{
		ID:     "r1",
		TaskID: "t1",
		Status: domain.AgentRunStatusFailed,
	}
	_, err := auth.Resolve(
		context.Background(),
		"tok",
		&mockAgentRunReader{run: run},
		&mockTaskReader{},
		&mockFlowLoader{},
	)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for failed run, got %v", err)
	}
}

func TestResolve_RunNotRunning_Interrupted(t *testing.T) {
	run := domain.AgentRun{
		ID:     "r1",
		TaskID: "t1",
		Status: domain.AgentRunStatusInterrupted,
	}
	_, err := auth.Resolve(
		context.Background(),
		"tok",
		&mockAgentRunReader{run: run},
		&mockTaskReader{},
		&mockFlowLoader{},
	)
	if !errors.Is(err, auth.ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized for interrupted run, got %v", err)
	}
}

func TestResolve_TaskNotFound(t *testing.T) {
	run := domain.AgentRun{
		ID:     "r1",
		TaskID: "t1",
		Status: domain.AgentRunStatusRunning,
	}
	taskErr := errors.New("task not found")
	_, err := auth.Resolve(
		context.Background(),
		"tok",
		&mockAgentRunReader{run: run},
		&mockTaskReader{err: taskErr},
		&mockFlowLoader{},
	)
	if !errors.Is(err, taskErr) {
		t.Fatalf("expected wrapped task error, got %v", err)
	}
}

func TestResolve_FlowLoadError(t *testing.T) {
	run := domain.AgentRun{
		ID:     "r1",
		TaskID: "t1",
		Status: domain.AgentRunStatusRunning,
	}
	task := domain.Task{
		ID:           "t1",
		CurrentState: "impl",
		FlowPath:     "custom.yaml",
	}
	flowErr := errors.New("flow file not found")
	_, err := auth.Resolve(
		context.Background(),
		"tok",
		&mockAgentRunReader{run: run},
		&mockTaskReader{task: task},
		&mockFlowLoader{err: flowErr},
	)
	if !errors.Is(err, flowErr) {
		t.Fatalf("expected wrapped flow error, got %v", err)
	}
}

func TestResolve_StateNotFoundInFlow(t *testing.T) {
	run := domain.AgentRun{
		ID:     "r1",
		TaskID: "t1",
		Status: domain.AgentRunStatusRunning,
	}
	task := domain.Task{
		ID:           "t1",
		CurrentState: "nonexistent_state",
		FlowPath:     "",
	}
	flowDef := validFlow("implementation", []string{"crowbar_signal"})
	_, err := auth.Resolve(
		context.Background(),
		"tok",
		&mockAgentRunReader{run: run},
		&mockTaskReader{task: task},
		&mockFlowLoader{def: flowDef},
	)
	if err == nil {
		t.Fatal("expected error for missing state, got nil")
	}
}

func TestResolve_HappyPath(t *testing.T) {
	run := domain.AgentRun{
		ID:     "run-42",
		TaskID: "task-99",
		Status: domain.AgentRunStatusRunning,
	}
	task := domain.Task{
		ID:           "task-99",
		CurrentState: "implementation",
		FlowPath:     "",
	}
	flowDef := validFlow("implementation", []string{"crowbar_signal"})

	actx, err := auth.Resolve(
		context.Background(),
		"good-token",
		&mockAgentRunReader{run: run},
		&mockTaskReader{task: task},
		&mockFlowLoader{def: flowDef},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if actx.AgentRun.ID != "run-42" {
		t.Errorf("expected run ID run-42, got %s", actx.AgentRun.ID)
	}
	if actx.Task.ID != "task-99" {
		t.Errorf("expected task ID task-99, got %s", actx.Task.ID)
	}
	if actx.State.Name != "implementation" {
		t.Errorf("expected state implementation, got %s", actx.State.Name)
	}
}

// --- HasTool tests ---

func TestHasTool_NilAgent(t *testing.T) {
	actx := auth.AgentContext{
		State: flow.StateDefinition{
			Name:  "human_review",
			Agent: nil,
		},
	}
	if auth.HasTool(actx, "crowbar_signal") {
		t.Fatal("expected false when Agent is nil")
	}
}

func TestHasTool_ToolPresent(t *testing.T) {
	actx := auth.AgentContext{
		State: flow.StateDefinition{
			Name: "implementation",
			Agent: &flow.AgentDef{
				Tools: []string{"crowbar_signal", "crowbar_create_item"},
			},
		},
	}
	if !auth.HasTool(actx, "crowbar_signal") {
		t.Fatal("expected true for crowbar_signal")
	}
}

func TestHasTool_ToolAbsent(t *testing.T) {
	actx := auth.AgentContext{
		State: flow.StateDefinition{
			Name: "implementation",
			Agent: &flow.AgentDef{
				Tools: []string{"crowbar_create_item"},
			},
		},
	}
	if auth.HasTool(actx, "crowbar_signal") {
		t.Fatal("expected false for crowbar_signal when not in list")
	}
}

func TestHasTool_EmptyTools(t *testing.T) {
	actx := auth.AgentContext{
		State: flow.StateDefinition{
			Name:  "implementation",
			Agent: &flow.AgentDef{Tools: []string{}},
		},
	}
	if auth.HasTool(actx, "crowbar_signal") {
		t.Fatal("expected false for empty tools list")
	}
}
