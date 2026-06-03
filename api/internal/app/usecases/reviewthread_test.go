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

func TestReviewThreadGet_Delegates(t *testing.T) {
	want := domain.ReviewThread{ID: "th1", File: "main.go"}
	repo := &mockReviewThreadRepo{}
	repo.getFn = func(_ context.Context, id string) (domain.ReviewThread, error) {
		return want, nil
	}
	uc := NewReviewThread(repo)
	got, err := uc.Get(context.Background(), "th1")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestReviewThreadGet_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &mockReviewThreadRepo{}
	repo.getFn = func(_ context.Context, id string) (domain.ReviewThread, error) {
		return domain.ReviewThread{}, boom
	}
	uc := NewReviewThread(repo)
	_, err := uc.Get(context.Background(), "nope")
	assert.ErrorIs(t, err, boom)
}

func TestReviewThreadOpen_PropagatesError(t *testing.T) {
	boom := errors.New("open error")
	repo := &mockReviewThreadRepo{
		openFn: func(_ context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy, content string) (domain.ReviewThread, error) {
			return domain.ReviewThread{}, boom
		},
	}
	uc := NewReviewThread(repo)
	_, err := uc.Open(context.Background(), "t1", "main.go", 10, "content")
	assert.ErrorIs(t, err, boom)
}

func TestReviewThreadPostMessage_PropagatesError(t *testing.T) {
	boom := errors.New("post error")
	repo := &mockReviewThreadRepo{
		postMessageFn: func(_ context.Context, threadID, role, content string) error {
			return boom
		},
	}
	uc := NewReviewThread(repo)
	assert.ErrorIs(t, uc.PostMessage(context.Background(), "th1", "human", "msg"), boom)
}

func TestReviewThreadForceApprove_PropagatesError(t *testing.T) {
	boom := errors.New("approve error")
	repo := &mockReviewThreadRepo{
		forceApproveFn: func(_ context.Context, threadID string) error {
			return boom
		},
	}
	uc := NewReviewThread(repo)
	assert.ErrorIs(t, uc.ForceApprove(context.Background(), "th1"), boom)
}

func TestReviewThreadList_Delegates(t *testing.T) {
	want := []domain.ReviewThread{{ID: "th1"}, {ID: "th2"}}
	repo := &mockReviewThreadRepo{
		listByTaskFn: func(_ context.Context, taskID, status, phase string) ([]domain.ReviewThread, error) {
			return want, nil
		},
	}
	uc := NewReviewThread(repo)
	got, err := uc.List(context.Background(), "t1", "open", "")
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestReviewThreadList_PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	repo := &mockReviewThreadRepo{
		listByTaskFn: func(_ context.Context, taskID, status, phase string) ([]domain.ReviewThread, error) {
			return nil, boom
		},
	}
	uc := NewReviewThread(repo)
	_, err := uc.List(context.Background(), "t1", "", "")
	assert.ErrorIs(t, err, boom)
}

func TestReviewThreadOpen_Delegates(t *testing.T) {
	want := domain.ReviewThread{ID: "th1", File: "main.go", Line: 10}
	var capturedPhase domain.ReviewPhase
	var capturedOpenedBy string
	repo := &mockReviewThreadRepo{
		openFn: func(_ context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy, content string) (domain.ReviewThread, error) {
			capturedPhase = phase
			capturedOpenedBy = openedBy
			return want, nil
		},
	}
	uc := NewReviewThread(repo)
	got, err := uc.Open(context.Background(), "t1", "main.go", 10, "needs fix")
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, domain.ReviewPhaseHumanReview, capturedPhase)
	assert.Equal(t, "human", capturedOpenedBy)
}

func TestReviewThreadPostMessage_Delegates(t *testing.T) {
	var capturedThread, capturedRole, capturedContent string
	repo := &mockReviewThreadRepo{
		postMessageFn: func(_ context.Context, threadID, role, content string) error {
			capturedThread = threadID
			capturedRole = role
			capturedContent = content
			return nil
		},
	}
	uc := NewReviewThread(repo)
	require.NoError(t, uc.PostMessage(context.Background(), "th1", "human", "looks good"))
	assert.Equal(t, "th1", capturedThread)
	assert.Equal(t, "human", capturedRole)
	assert.Equal(t, "looks good", capturedContent)
}

func TestReviewThreadForceApprove_Delegates(t *testing.T) {
	var called string
	repo := &mockReviewThreadRepo{
		forceApproveFn: func(_ context.Context, threadID string) error {
			called = threadID
			return nil
		},
	}
	uc := NewReviewThread(repo)
	require.NoError(t, uc.ForceApprove(context.Background(), "th1"))
	assert.Equal(t, "th1", called)
}

// ---------- mock ReviewThread repo ----------

type mockReviewThreadRepo struct {
	getFn          func(ctx context.Context, id string) (domain.ReviewThread, error)
	openFn          func(ctx context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy, content string) (domain.ReviewThread, error)
	listByTaskFn    func(ctx context.Context, taskID, status, phase string) ([]domain.ReviewThread, error)
	postMessageFn   func(ctx context.Context, threadID, role, content string) error
	forceApproveFn  func(ctx context.Context, threadID string) error
}

func (m *mockReviewThreadRepo) Open(ctx context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy, content string) (domain.ReviewThread, error) {
	if m.openFn != nil {
		return m.openFn(ctx, taskID, agentRunID, file, line, phase, openedBy, content)
	}
	return domain.ReviewThread{}, nil
}
func (m *mockReviewThreadRepo) Get(ctx context.Context, id string) (domain.ReviewThread, error) {
	if m.getFn != nil {
		return m.getFn(ctx, id)
	}
	return domain.ReviewThread{}, nil
}
func (m *mockReviewThreadRepo) ListByTask(ctx context.Context, taskID, status, phase string) ([]domain.ReviewThread, error) {
	if m.listByTaskFn != nil {
		return m.listByTaskFn(ctx, taskID, status, phase)
	}
	return nil, nil
}
func (m *mockReviewThreadRepo) PostMessage(ctx context.Context, threadID, role, content string) error {
	if m.postMessageFn != nil {
		return m.postMessageFn(ctx, threadID, role, content)
	}
	return nil
}
func (m *mockReviewThreadRepo) ForceApprove(ctx context.Context, threadID string) error {
	if m.forceApproveFn != nil {
		return m.forceApproveFn(ctx, threadID)
	}
	return nil
}
func (m *mockReviewThreadRepo) ResolveThread(_ context.Context, _, _ string) error { return nil }
func (m *mockReviewThreadRepo) Subscribe(_ string, _ func(context.Context, asynxModels.Event[domain.ReviewThread])) (string, error) {
	return "", nil
}
