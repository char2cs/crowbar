// Package chat (file placement_reads.go) serves the PLACEMENT reads of the
// runner projections: which CLI is on a chat right now, and which conversations
// it has hosted. Nothing here starts, stops or moves a process.
package chat

import (
	"context"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// LiveRunnerForChat returns the CLI currently placed on the chat.
func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (engineagents.Runner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

// ConversationsForChat returns every conversation a CLI has hosted on the chat,
// oldest first.
func (u *Usecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]engineagents.ChatConversation, error) {
	return u.runners.ConversationsForChat(ctx, chatID)
}
