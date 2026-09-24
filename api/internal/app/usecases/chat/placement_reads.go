// Package chat (file placement_reads.go) serves the three PLACEMENT reads of the
// runner projections: which CLI is on a chat right now, which conversations it
// has hosted, and which providers have ever been placed on it.
//
// They are together, and apart from the runner lifecycle in runner.go, because
// they answer one question between them — "what has run here" — and nothing here
// starts, stops or moves a process. The first is live state and the other two are
// append-only history, which is exactly why a dormant chat can still be asked.
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

// PlacementsForChat returns every provider a runner has ever been placed on the
// chat as, oldest arrival first — the append-only record that still names a
// dormant chat's vendor when its provider announced no conversation to fall back
// to and it was never switched.
func (u *Usecase) PlacementsForChat(
	ctx context.Context,
	chatID string,
) ([]engineagents.ChatPlacement, error) {
	return u.runners.PlacementsForChat(ctx, chatID)
}
