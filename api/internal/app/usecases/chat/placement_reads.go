// Package chat (file placement_reads.go) serves the placement reads: which CLI
// is on a chat right now, and which conversations the chat has bound. Nothing
// here starts or stops a process.
package chat

import (
	"context"
	"errors"
	"fmt"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// LiveRunnerForChat returns the CLI on the chat; ErrNoLiveRunner when it is
// dormant.
func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (engineagents.Runner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

// RunnerByID is the live runner runnerID, also while it is exiting: the MCP
// resolver's port.
func (u *Usecase) RunnerByID(runnerID string) (engineagents.Runner, bool) {
	return u.runners.RunnerByID(runnerID)
}

// RelayRunner is the live runner on the chat whose bound session holds relayID: the MCP
// resolver's port for a relay that outlives its runners. A lost, replaced or moved-away
// binding holds nothing, and neither does a chat with no runner now.
func (u *Usecase) RelayRunner(ctx context.Context, relayID string) (engineagents.Runner, bool, error) {
	chatID, ok, err := u.ledger.RelayOwner(ctx, relayID)
	if err != nil || !ok {
		return engineagents.Runner{}, false, err
	}
	st, err := u.ledger.State(ctx, chatID)
	if err != nil {
		return engineagents.Runner{}, false, fmt.Errorf("agent: relay runner: %w", err)
	}
	bound, ok := st.RelayConversation(relayID)
	if !ok {
		return engineagents.Runner{}, false, nil
	}
	live, err := u.runners.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, ErrNoLiveRunner) {
		return engineagents.Runner{}, false, nil
	}
	if err != nil {
		return engineagents.Runner{}, false, err
	}
	return live, live.ProviderID == bound.Provider, nil
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
