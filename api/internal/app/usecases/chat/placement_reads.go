// Package chat (file placement_reads.go) serves the placement reads: which CLI
// is on a chat right now, and which conversations the chat has bound. Nothing
// here starts or stops a process.
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

// ConversationsForChat returns every (provider, session) the chat's ledger
// has bound, oldest first.
func (u *Usecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]engineagents.ChatConversation, error) {
	snap, err := u.snapshots.Get(ctx, chatID)
	if err != nil {
		return nil, err
	}
	out := make([]engineagents.ChatConversation, 0, len(snap.Ledger.Conversations))
	for _, c := range snap.Ledger.Conversations {
		out = append(out, engineagents.ChatConversation{
			ChatID: chatID, ProviderID: c.Provider, SessionID: c.SessionID,
			FirstSeenAt: c.BoundAt, LastActiveAt: c.BoundAt,
		})
	}
	return out, nil
}
