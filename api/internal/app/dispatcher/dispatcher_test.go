package dispatcher

import (
	"context"
	"errors"
	"testing"
	"time"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/internal/engine/flow"
)

// ---------- helpers ----------

func makeEvent(t domain.Task) asynxModels.Event[domain.Task] {
	return asynxModels.Event[domain.Task]{Aggregate: t}
}

func pendingTask(state string) domain.Task {
	return domain.Task{
		ID:           "t-1",
		CurrentState: state,
		Status:       domain.TaskStatusPending,
		FlowPath:     "",
	}
}

// ---------- tests ----------

func TestHandleEvent_NilRuntime_Skips(t *testing.T) {
	created := false
	d := New(context.Background(), nil,
		&mockAgentRunRepo{
			createFn: func(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
				created = true
				return domain.AgentRun{}, nil
			},
		},
		&mockFlowLoader{},
		nil, // nil runtime
	)

	d.handleEvent(context.Background(), makeEvent(pendingTask("brainstorming")))
	assert.False(t, created)
}

func TestHandleEvent_ArchivedTask_Skips(t *testing.T) {
	created := false
	d := New(context.Background(), nil,
		&mockAgentRunRepo{
			createFn: func(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
				created = true
				return domain.AgentRun{}, nil
			},
		},
		&mockFlowLoader{},
		&mockRuntime{},
	)

	task := pendingTask("brainstorming")
	task.Status = domain.TaskStatusArchived
	d.handleEvent(context.Background(), makeEvent(task))
	assert.False(t, created)
}

func TestHandleEvent_PausedTask_Skips(t *testing.T) {
	created := false
	d := New(context.Background(), nil,
		&mockAgentRunRepo{
			createFn: func(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
				created = true
				return domain.AgentRun{}, nil
			},
		},
		&mockFlowLoader{},
		&mockRuntime{},
	)

	task := pendingTask("brainstorming")
	task.Status = domain.TaskStatusPaused
	d.handleEvent(context.Background(), makeEvent(task))
	assert.False(t, created)
}

func TestHandleEvent_HumanState_Skips(t *testing.T) {
	// "human_review" in the builtin flow has Agent == nil.
	created := false
	d := New(context.Background(), nil,
		&mockAgentRunRepo{
			createFn: func(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
				created = true
				return domain.AgentRun{}, nil
			},
		},
		flow.NewLoader(),
		&mockRuntime{},
	)

	d.handleEvent(context.Background(), makeEvent(pendingTask("human_review")))
	assert.False(t, created)
}

func TestHandleEvent_UnknownState_Skips(t *testing.T) {
	created := false
	d := New(context.Background(), nil,
		&mockAgentRunRepo{
			createFn: func(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
				created = true
				return domain.AgentRun{}, nil
			},
		},
		flow.NewLoader(),
		&mockRuntime{},
	)

	d.handleEvent(context.Background(), makeEvent(pendingTask("nonexistent_state")))
	assert.False(t, created)
}

func TestHandleEvent_AgentState_Dispatches(t *testing.T) {
	runCalled := make(chan domain.AgentRun, 1)

	d := New(context.Background(), nil,
		&mockAgentRunRepo{
			createFn: func(_ context.Context, taskID, stateName, _ string) (domain.AgentRun, error) {
				return domain.AgentRun{ID: "run-1", TaskID: taskID, StateName: stateName}, nil
			},
		},
		flow.NewLoader(),
		&mockRuntime{
			runFn: func(_ context.Context, run domain.AgentRun, _ domain.Task, _ flow.StateDefinition) error {
				runCalled <- run
				return nil
			},
		},
	)

	d.handleEvent(context.Background(), makeEvent(pendingTask("brainstorming")))

	select {
	case run := <-runCalled:
		assert.Equal(t, "run-1", run.ID)
		assert.Equal(t, "t-1", run.TaskID)
		assert.Equal(t, "brainstorming", run.StateName)
	case <-time.After(time.Second):
		t.Fatal("timeout: runtime.Run was not called")
	}
}

func TestHandleEvent_RuntimeFails_FailsAgentRun(t *testing.T) {
	failCalled := make(chan string, 1)

	d := New(context.Background(), nil,
		&mockAgentRunRepo{
			createFn: func(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
				return domain.AgentRun{ID: "run-2"}, nil
			},
			failFn: func(_ context.Context, id string) error {
				failCalled <- id
				return nil
			},
		},
		flow.NewLoader(),
		&mockRuntime{
			runFn: func(_ context.Context, _ domain.AgentRun, _ domain.Task, _ flow.StateDefinition) error {
				return errors.New("agent crashed")
			},
		},
	)

	d.handleEvent(context.Background(), makeEvent(pendingTask("brainstorming")))

	select {
	case id := <-failCalled:
		assert.Equal(t, "run-2", id)
	case <-time.After(time.Second):
		t.Fatal("timeout: FailAgentRun was not called after runtime error")
	}
}

func TestRegister_WiresSubscription(t *testing.T) {
	var capturedTopic string
	taskRepo := &mockTaskRepo{
		subscribeFn: func(topic string, _ func(context.Context, asynxModels.Event[domain.Task])) (string, error) {
			capturedTopic = topic
			return "sub-1", nil
		},
	}

	d := New(context.Background(), taskRepo, &mockAgentRunRepo{}, &mockFlowLoader{}, nil)
	require.NoError(t, d.Register())
	assert.Equal(t, "task.*", capturedTopic)
}

// ---------- mock task repo ----------

type mockTaskRepo struct {
	subscribeFn func(topic string, fn func(context.Context, asynxModels.Event[domain.Task])) (string, error)
}

func (m *mockTaskRepo) Create(_ context.Context, _, _, _, _, _, _, _, _ string) (domain.Task, error) {
	return domain.Task{}, nil
}
func (m *mockTaskRepo) Get(_ context.Context, _ string) (domain.Task, error) {
	return domain.Task{}, nil
}
func (m *mockTaskRepo) List(_ context.Context, _ string) ([]domain.Task, error) {
	return nil, nil
}
func (m *mockTaskRepo) Archive(_ context.Context, _ string) error  { return nil }
func (m *mockTaskRepo) Pause(_ context.Context, _ string) error    { return nil }
func (m *mockTaskRepo) Resume(_ context.Context, _ string) error   { return nil }
func (m *mockTaskRepo) Retry(_ context.Context, _ string) error    { return nil }
func (m *mockTaskRepo) AdvanceState(_ context.Context, _, _, _ string) error {
	return nil
}
func (m *mockTaskRepo) ForceTransition(_ context.Context, _, _ string) error { return nil }
func (m *mockTaskRepo) Subscribe(topic string, fn func(context.Context, asynxModels.Event[domain.Task])) (string, error) {
	if m.subscribeFn != nil {
		return m.subscribeFn(topic, fn)
	}
	return "", nil
}

// ---------- mock agent run repo ----------

type mockAgentRunRepo struct {
	createFn func(ctx context.Context, taskID, stateName, token string) (domain.AgentRun, error)
	failFn   func(ctx context.Context, id string) error
}

func (m *mockAgentRunRepo) Create(ctx context.Context, taskID, stateName, token string) (domain.AgentRun, error) {
	if m.createFn != nil {
		return m.createFn(ctx, taskID, stateName, token)
	}
	return domain.AgentRun{}, nil
}
func (m *mockAgentRunRepo) Get(_ context.Context, _ string) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}
func (m *mockAgentRunRepo) GetByToken(_ context.Context, _ string) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}
func (m *mockAgentRunRepo) ListByTask(_ context.Context, _ string) ([]domain.AgentRun, error) {
	return nil, nil
}
func (m *mockAgentRunRepo) CompleteAgentRun(_ context.Context, _, _ string) error { return nil }
func (m *mockAgentRunRepo) FailAgentRun(ctx context.Context, id string) error {
	if m.failFn != nil {
		return m.failFn(ctx, id)
	}
	return nil
}
func (m *mockAgentRunRepo) InterruptAgentRun(_ context.Context, _ string) error { return nil }
func (m *mockAgentRunRepo) RecoverOrphanedRuns(_ context.Context) error         { return nil }
func (m *mockAgentRunRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.AgentRun])) (string, error) {
	return "", nil
}

// ---------- mock flow loader ----------

type mockFlowLoader struct {
	loadFn func(flowPath string) (flow.FlowDefinition, error)
}

func (m *mockFlowLoader) Load(flowPath string) (flow.FlowDefinition, error) {
	if m.loadFn != nil {
		return m.loadFn(flowPath)
	}
	return flow.FlowDefinition{}, nil
}

// ---------- mock runtime ----------

type mockRuntime struct {
	runFn func(ctx context.Context, run domain.AgentRun, task domain.Task, state flow.StateDefinition) error
}

func (m *mockRuntime) Run(ctx context.Context, run domain.AgentRun, task domain.Task, state flow.StateDefinition) error {
	if m.runFn != nil {
		return m.runFn(ctx, run, task, state)
	}
	return nil
}

var _ agent.AgentRuntime = (*mockRuntime)(nil)
