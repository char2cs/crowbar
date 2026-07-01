package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// MockChat is a test double for chat.Chat. Each method delegates to the
// corresponding function-field, allowing callers to inject behaviour per-test
// without subclassing or code generation.
type MockChat struct {
	CreateFn          func(ctx context.Context, id, wsID, title string, now time.Time) (domain.Chat, error)
	ForkFn            func(ctx context.Context, id, wsID, parentID, title string, now time.Time) (domain.Chat, error)
	RenameFn          func(ctx context.Context, id, title string) (domain.Chat, error)
	DeleteFn          func(ctx context.Context, id string, now time.Time) (domain.Chat, error)
	GetFn             func(ctx context.Context, id string) (domain.Chat, error)
	ListFn            func(ctx context.Context) ([]domain.Chat, error)
	ListByWorkspaceFn func(ctx context.Context, wsID string) ([]domain.Chat, error)
}

func (m *MockChat) Create(
	ctx context.Context,
	id string,
	wsID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	return m.CreateFn(ctx, id, wsID, title, now)
}

func (m *MockChat) Fork(
	ctx context.Context,
	id string,
	wsID string,
	parentID string,
	title string,
	now time.Time,
) (domain.Chat, error) {
	return m.ForkFn(ctx, id, wsID, parentID, title, now)
}

func (m *MockChat) Rename(
	ctx context.Context,
	id string,
	title string,
) (domain.Chat, error) {
	return m.RenameFn(ctx, id, title)
}

func (m *MockChat) Delete(
	ctx context.Context,
	id string,
	now time.Time,
) (domain.Chat, error) {
	return m.DeleteFn(ctx, id, now)
}

func (m *MockChat) Get(
	ctx context.Context,
	id string,
) (domain.Chat, error) {
	return m.GetFn(ctx, id)
}

func (m *MockChat) List(
	ctx context.Context,
) ([]domain.Chat, error) {
	return m.ListFn(ctx)
}

func (m *MockChat) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.Chat, error) {
	return m.ListByWorkspaceFn(ctx, wsID)
}

var _ chat.Chat = (*MockChat)(nil)
