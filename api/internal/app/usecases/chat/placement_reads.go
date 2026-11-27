package chat

import (
	"context"
	"errors"
	"fmt"

	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (engineagents.Runner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

func (u *Usecase) RunnerByID(runnerID string) (engineagents.Runner, bool) {
	return u.runners.RunnerByID(runnerID)
}

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
