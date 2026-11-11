package usecases

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// chatGetter is the minimal chat-read surface agentChatReader adapts.
type chatGetter interface {
	GetChat(ctx context.Context, id string) (domain.Chat, error)
	ListChats(ctx context.Context) ([]domain.Chat, error)
}

// agentChatReader adapts the chat repository into agentusecase.ToolChatReader. Only
// the name differs: the repository says GetChat because it also serves runners,
// while the tool surface's port is a plain Get.
type agentChatReader struct {
	chats chatGetter
}

// Get implements agentusecase.ToolChatReader.
func (r agentChatReader) Get(
	ctx context.Context,
	chatID string,
) (domain.Chat, error) {
	return r.chats.GetChat(ctx, chatID)
}

// ListChats implements agentusecase.ToolChatReader.
func (r agentChatReader) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return r.chats.ListChats(ctx)
}
