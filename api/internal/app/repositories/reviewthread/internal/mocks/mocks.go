package mocks

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// MockReviewThread is a test double for reviewthread.ReviewThread.
type MockReviewThread struct {
	OpenFn            func(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error)
	ReplyFn           func(ctx context.Context, id, messageID, author string, isAgent bool, body string, now time.Time) (domain.ReviewThread, error)
	ResolveFn         func(ctx context.Context, id string) (domain.ReviewThread, error)
	ReopenFn          func(ctx context.Context, id string) (domain.ReviewThread, error)
	GetFn             func(ctx context.Context, id string) (domain.ReviewThread, error)
	ListFn            func(ctx context.Context) ([]domain.ReviewThread, error)
	ListByWorkspaceFn func(ctx context.Context, wsID string) ([]domain.ReviewThread, error)
}

func (m *MockReviewThread) Open(
	ctx context.Context,
	in reviewthread.OpenInput,
	now time.Time,
) (domain.ReviewThread, error) {
	return m.OpenFn(ctx, in, now)
}

func (m *MockReviewThread) Reply(
	ctx context.Context,
	id string,
	messageID string,
	author string,
	isAgent bool,
	body string,
	now time.Time,
) (domain.ReviewThread, error) {
	return m.ReplyFn(ctx, id, messageID, author, isAgent, body, now)
}

func (m *MockReviewThread) Resolve(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	return m.ResolveFn(ctx, id)
}

func (m *MockReviewThread) Reopen(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	return m.ReopenFn(ctx, id)
}

func (m *MockReviewThread) Get(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	return m.GetFn(ctx, id)
}

func (m *MockReviewThread) List(
	ctx context.Context,
) ([]domain.ReviewThread, error) {
	return m.ListFn(ctx)
}

func (m *MockReviewThread) ListByWorkspace(
	ctx context.Context,
	wsID string,
) ([]domain.ReviewThread, error) {
	return m.ListByWorkspaceFn(ctx, wsID)
}

var _ reviewthread.ReviewThread = (*MockReviewThread)(nil)
