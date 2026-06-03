package usecases

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ConversationUsecase is the public contract for conversation message operations.
type ConversationUsecase interface {
	List(
		ctx context.Context,
		taskID string,
	) ([]domain.ConversationMessage, error)
}

type conversationUsecase struct {
	repo repositories.Conversation
}

// NewConversation wires a Conversation repository into a ConversationUsecase.
func NewConversation(
	repo repositories.Conversation,
) ConversationUsecase {
	return &conversationUsecase{repo: repo}
}

func (u *conversationUsecase) List(
	ctx context.Context,
	taskID string,
) ([]domain.ConversationMessage, error) {
	return u.repo.ListByTask(ctx, taskID)
}
