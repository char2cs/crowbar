package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestConversationList_Delegates(t *testing.T) {
	want := []domain.ConversationMessage{{ID: "m1"}, {ID: "m2"}}
	repo := &mockConversationRepo{
		listByTaskFn: func(_ context.Context, taskID string) ([]domain.ConversationMessage, error) {
			return want, nil
		},
	}
	uc := NewConversation(repo)
	got, err := uc.List(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestConversationList_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &mockConversationRepo{
		listByTaskFn: func(_ context.Context, taskID string) ([]domain.ConversationMessage, error) {
			return nil, boom
		},
	}
	uc := NewConversation(repo)
	_, err := uc.List(context.Background(), "t1")
	assert.ErrorIs(t, err, boom)
}

// ---------- mock Conversation repo ----------

type mockConversationRepo struct {
	listByTaskFn func(ctx context.Context, taskID string) ([]domain.ConversationMessage, error)
}

func (m *mockConversationRepo) Create(_ context.Context, _ string, _ domain.ConversationMessageRole, _ domain.ConversationMessageType, _ string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}
func (m *mockConversationRepo) ListByTask(ctx context.Context, taskID string) ([]domain.ConversationMessage, error) {
	if m.listByTaskFn != nil {
		return m.listByTaskFn(ctx, taskID)
	}
	return nil, nil
}
