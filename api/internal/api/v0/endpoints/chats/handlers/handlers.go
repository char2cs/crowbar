// Package handlers holds the gin handlers backing the chats endpoint: the flat
// per-workspace list and the lifecycle mutations (create root, fork from tip,
// rename, and cascade delete).
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Lifecycle is the chat usecase surface the handlers need: the flat
// per-workspace read plus the create/fork/rename/cascade-delete mutations.
type Lifecycle interface {
	ListChatsByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
	CreateChat(
		ctx context.Context,
		id string,
		wsID string,
		title string,
		now time.Time,
	) (domain.Chat, error)
	ForkChat(
		ctx context.Context,
		parentID string,
		now time.Time,
	) (domain.Chat, error)
	RenameChat(
		ctx context.Context,
		id string,
		title string,
	) (domain.Chat, error)
	DeleteChat(
		ctx context.Context,
		id string,
		now time.Time,
	) error
}

// Handlers serves the /v0 chat routes from the chat lifecycle usecase. now
// supplies the timestamp for the create, fork, and delete activity roll-ups.
type Handlers struct {
	chats Lifecycle
	now   func() time.Time
}

// New builds the chats Handlers from the chat lifecycle usecase and a clock.
func New(
	chats Lifecycle,
	now func() time.Time,
) *Handlers {
	return &Handlers{
		chats: chats,
		now:   now,
	}
}
