package tools

import (
	"context"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"

	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The seams the review tools read and write the code-review record through.

// ReviewReader is the read port onto the branch-review usecase: a review's scope (base ref, files and
// geometry from one git resolution) and a diff's hunk outline. GetScope takes the resolved workspace,
// not its id, so the aggregate is not folded twice.
type ReviewReader interface {
	GetScope(ctx context.Context, ws domain.Workspace) (gitdomain.ReviewScope, error)
	GetOutline(ctx context.Context, wsID, commit string) ([]gitdomain.FileOutline, error)
}

// ThreadReader is the read port onto the review-thread store. Every tool acting on a named thread calls
// Get first: CanSee is checked against the WsID it returns, never the id argument alone.
type ThreadReader interface {
	ListByWorkspace(ctx context.Context, wsID string) ([]domain.ReviewThread, error)
	Get(ctx context.Context, id string) (domain.ReviewThread, error)
}

// ThreadBroadcast announces an agent-written review thread to connected clients; the thread store does
// not fan out, so without it the write stays invisible. It takes the domain aggregate plus project and
// repo ids (the /threads stream filters on them); the wiring layer converts to the wire DTO.
type ThreadBroadcast func(
	thread domain.ReviewThread,
	projectID string,
	repoID string,
)

// ThreadWriter is the write half of the review-thread port, matching the reviewthread store's own
// Open/Reply/Resolve signatures so the store satisfies it with no adapter.
type ThreadWriter interface {
	Open(ctx context.Context, in reviewthread.OpenInput, now time.Time) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		in reviewthread.ReplyInput,
		now time.Time,
	) (domain.ReviewThread, error)
	Resolve(ctx context.Context, id string) (domain.ReviewThread, error)
}

// ThreadChanges wakes a waiter when a review thread of one workspace changes. The
// channel carries the thread aggregate as the event left it; cancel must be called
// once the waiter is done.
type ThreadChanges interface {
	Watch(wsID string) (<-chan domain.ReviewThread, func())
}
