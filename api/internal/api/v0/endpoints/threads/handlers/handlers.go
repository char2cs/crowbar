package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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

type ThreadBroadcaster interface {
	Push(
		d dto.ThreadDTO,
	)
}

type Handlers struct {
	store     ThreadStore
	broadcast ThreadBroadcaster
	now       func() time.Time
	newID     func() string
}

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
