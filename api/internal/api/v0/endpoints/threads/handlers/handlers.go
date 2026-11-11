// Package handlers serves the workspace-scoped review-thread routes over the ReviewThread
// aggregate. The owning project/repo come from the request path and are stamped onto each
// ThreadDTO before it is broadcast.
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ThreadStore is the subset of reviewthread.ReviewThread the handlers consume; the concrete
// repository satisfies it directly.
type ThreadStore interface {
	Open(
		ctx context.Context,
		in reviewthread.OpenInput,
		now time.Time,
	) (domain.ReviewThread, error)
	Reply(
		ctx context.Context,
		in reviewthread.ReplyInput,
		now time.Time,
	) (domain.ReviewThread, error)
	EditMessage(
		ctx context.Context,
		id string,
		messageID string,
		body string,
	) (domain.ReviewThread, error)
	DeleteMessage(
		ctx context.Context,
		id string,
		messageID string,
	) (domain.ReviewThread, error)
	DeleteThread(
		ctx context.Context,
		id string,
	) error
	Resolve(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
	Reopen(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
	Get(
		ctx context.Context,
		id string,
	) (domain.ReviewThread, error)
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.ReviewThread, error)
}

// ThreadBroadcaster receives workspace-scoped thread DTOs so the v0
// Broadcaster[ThreadDTO] can fan them out to subscribed clients. Threads are
// sync-mutate → broadcast: open/reply/resolve push the updated DTO through it.
type ThreadBroadcaster interface {
	Push(
		d dto.ThreadDTO,
	)
}

// Handlers serves the /v0 workspace-scoped thread routes from the review-thread
// aggregate, broadcasting every mutation through the thread broadcaster.
type Handlers struct {
	store     ThreadStore
	broadcast ThreadBroadcaster
	now       func() time.Time
	newID     func() string
}

// New builds the thread Handlers over the review-thread store and the thread
// broadcaster. The clock and id factory default to the wall clock and UUIDs and
// are only overridden in tests.
func New(
	store ThreadStore,
	broadcast ThreadBroadcaster,
) *Handlers {
	return &Handlers{
		store:     store,
		broadcast: broadcast,
		now:       func() time.Time { return time.Now().UTC() },
		newID:     newUUID,
	}
}
