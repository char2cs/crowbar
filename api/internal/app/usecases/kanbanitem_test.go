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

func TestKanbanItemList_Delegates(t *testing.T) {
	want := []domain.KanbanItem{{ID: "k1"}, {ID: "k2"}}
	repo := &mockKanbanRepo{
		listByTaskFn: func(_ context.Context, taskID string) ([]domain.KanbanItem, error) {
			return want, nil
		},
	}
	uc := NewKanbanItem(repo)
	got, err := uc.List(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestKanbanItemList_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &mockKanbanRepo{
		listByTaskFn: func(_ context.Context, taskID string) ([]domain.KanbanItem, error) {
			return nil, boom
		},
	}
	uc := NewKanbanItem(repo)
	_, err := uc.List(context.Background(), "t1")
	assert.ErrorIs(t, err, boom)
}

func TestKanbanItemCreate_Delegates(t *testing.T) {
	want := domain.KanbanItem{ID: "k1", TaskID: "t1", Title: "do it"}
	repo := &mockKanbanRepo{
		createFn: func(_ context.Context, taskID, title, agentRunID string) (domain.KanbanItem, error) {
			return want, nil
		},
	}
	uc := NewKanbanItem(repo)
	got, err := uc.Create(context.Background(), "t1", "do it")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestKanbanItemUpdateStatus_Delegates(t *testing.T) {
	var capturedID, capturedStatus string
	repo := &mockKanbanRepo{
		updateStatusFn: func(_ context.Context, id, status, agentRunID string) error {
			capturedID = id
			capturedStatus = status
			return nil
		},
	}
	uc := NewKanbanItem(repo)
	require.NoError(t, uc.UpdateStatus(context.Background(), "k1", "done"))
	assert.Equal(t, "k1", capturedID)
	assert.Equal(t, "done", capturedStatus)
}

// ---------- mock KanbanItem repo ----------

type mockKanbanRepo struct {
	createFn       func(ctx context.Context, taskID, title, agentRunID string) (domain.KanbanItem, error)
	listByTaskFn   func(ctx context.Context, taskID string) ([]domain.KanbanItem, error)
	updateStatusFn func(ctx context.Context, id, status, agentRunID string) error
}

func (m *mockKanbanRepo) Create(ctx context.Context, taskID, title, agentRunID string) (domain.KanbanItem, error) {
	if m.createFn != nil {
		return m.createFn(ctx, taskID, title, agentRunID)
	}
	return domain.KanbanItem{}, nil
}
func (m *mockKanbanRepo) Get(_ context.Context, _ string) (domain.KanbanItem, error) {
	return domain.KanbanItem{}, nil
}
func (m *mockKanbanRepo) ListByTask(ctx context.Context, taskID string) ([]domain.KanbanItem, error) {
	if m.listByTaskFn != nil {
		return m.listByTaskFn(ctx, taskID)
	}
	return nil, nil
}
func (m *mockKanbanRepo) UpdateStatus(ctx context.Context, id, status, agentRunID string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, status, agentRunID)
	}
	return nil
}
func (m *mockKanbanRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.KanbanItem])) (string, error) {
	return "", nil
}
