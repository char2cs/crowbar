package usecases

import (
	"context"
	"errors"
	"testing"

	asynxModels "github.com/char2cs/asynx/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestAgentRunList_Delegates(t *testing.T) {
	want := []domain.AgentRun{{ID: "a1"}, {ID: "a2"}}
	repo := &mockAgentRunRepo{
		listByTaskFn: func(_ context.Context, taskID string) ([]domain.AgentRun, error) {
			return want, nil
		},
	}
	uc := NewAgentRun(repo)
	got, err := uc.List(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestAgentRunList_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &mockAgentRunRepo{
		listByTaskFn: func(_ context.Context, taskID string) ([]domain.AgentRun, error) {
			return nil, boom
		},
	}
	uc := NewAgentRun(repo)
	_, err := uc.List(context.Background(), "t1")
	assert.ErrorIs(t, err, boom)
}

func TestAgentRunInterrupt_Delegates(t *testing.T) {
	var called string
	repo := &mockAgentRunRepo{
		interruptFn: func(_ context.Context, id string) error {
			called = id
			return nil
		},
	}
	uc := NewAgentRun(repo)
	require.NoError(t, uc.Interrupt(context.Background(), "a1"))
	assert.Equal(t, "a1", called)
}

func TestAgentRunInterrupt_PropagatesError(t *testing.T) {
	repo := &mockAgentRunRepo{
		interruptFn: func(_ context.Context, id string) error {
			return domain.ErrNotFound
		},
	}
	uc := NewAgentRun(repo)
	assert.ErrorIs(t, uc.Interrupt(context.Background(), "nope"), domain.ErrNotFound)
}

// ---------- mock AgentRun repo ----------

type mockAgentRunRepo struct {
	listByTaskFn func(ctx context.Context, taskID string) ([]domain.AgentRun, error)
	interruptFn  func(ctx context.Context, id string) error
}

func (m *mockAgentRunRepo) Create(_ context.Context, _, _, _ string) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}
func (m *mockAgentRunRepo) Get(_ context.Context, _ string) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}
func (m *mockAgentRunRepo) GetByToken(_ context.Context, _ string) (domain.AgentRun, error) {
	return domain.AgentRun{}, nil
}
func (m *mockAgentRunRepo) ListByTask(ctx context.Context, taskID string) ([]domain.AgentRun, error) {
	if m.listByTaskFn != nil {
		return m.listByTaskFn(ctx, taskID)
	}
	return nil, nil
}
func (m *mockAgentRunRepo) CompleteAgentRun(_ context.Context, _, _ string) error { return nil }
func (m *mockAgentRunRepo) FailAgentRun(_ context.Context, _ string) error        { return nil }
func (m *mockAgentRunRepo) InterruptAgentRun(ctx context.Context, id string) error {
	if m.interruptFn != nil {
		return m.interruptFn(ctx, id)
	}
	return nil
}
func (m *mockAgentRunRepo) RecoverOrphanedRuns(_ context.Context) error { return nil }
func (m *mockAgentRunRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.AgentRun])) (string, error) {
	return "", nil
}
