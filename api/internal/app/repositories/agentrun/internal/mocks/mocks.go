package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrun"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// MockAgentRun is a test double for agentrun.AgentRun. Each method delegates to
// the corresponding function-field, allowing callers to inject behaviour per-test
// without subclassing or code generation.
type MockAgentRun struct {
	CreateFn      func(ctx context.Context, id, wsID, chatID string, now time.Time) (domain.AgentRun, error)
	MarkRunningFn func(ctx context.Context, id string) (domain.AgentRun, error)
	CompleteFn    func(ctx context.Context, id string) (domain.AgentRun, error)
	FailFn        func(ctx context.Context, id string) (domain.AgentRun, error)
	GetFn         func(ctx context.Context, id string) (domain.AgentRun, error)
	ListFn        func(ctx context.Context) ([]domain.AgentRun, error)
	ListRunningFn func(ctx context.Context) ([]domain.AgentRun, error)
	ListByChatFn  func(ctx context.Context, chatID string) ([]domain.AgentRun, error)
}

func (m *MockAgentRun) Create(
	ctx context.Context,
	id string,
	wsID string,
	chatID string,
	now time.Time,
) (domain.AgentRun, error) {
	return m.CreateFn(ctx, id, wsID, chatID, now)
}

func (m *MockAgentRun) MarkRunning(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	return m.MarkRunningFn(ctx, id)
}

func (m *MockAgentRun) Complete(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	return m.CompleteFn(ctx, id)
}

func (m *MockAgentRun) Fail(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	return m.FailFn(ctx, id)
}

func (m *MockAgentRun) Get(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	return m.GetFn(ctx, id)
}

func (m *MockAgentRun) List(
	ctx context.Context,
) ([]domain.AgentRun, error) {
	return m.ListFn(ctx)
}

func (m *MockAgentRun) ListRunning(
	ctx context.Context,
) ([]domain.AgentRun, error) {
	return m.ListRunningFn(ctx)
}

func (m *MockAgentRun) ListByChat(
	ctx context.Context,
	chatID string,
) ([]domain.AgentRun, error) {
	return m.ListByChatFn(ctx, chatID)
}

var _ agentrun.AgentRun = (*MockAgentRun)(nil)
