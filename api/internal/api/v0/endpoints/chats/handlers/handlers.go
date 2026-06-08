// Package handlers holds the gin handlers backing the chats endpoint.
package handlers

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ChatUsecase is the chat lifecycle surface the handlers need.
type ChatUsecase interface {
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

// ChatRepo is the chat read surface the handlers need.
type ChatRepo interface {
	ListByWorkspace(
		ctx context.Context,
		wsID string,
	) ([]domain.Chat, error)
}

// Handlers serves the /v0/chats routes from the chat usecase and repo.
type Handlers struct {
	chatUsecase ChatUsecase
	chatRepo    ChatRepo
}

// New builds the chats Handlers from the chat usecase and repo.
func New(
	chatUsecase ChatUsecase,
	chatRepo ChatRepo,
) *Handlers {
	return &Handlers{
		chatUsecase: chatUsecase,
		chatRepo:    chatRepo,
	}
}
