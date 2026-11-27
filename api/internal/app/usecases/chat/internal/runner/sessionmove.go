package runner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) claimHeld(ctx context.Context, chatID, providerID, sessionID, relayID string) ([]ledger.Row, error) {
	owner, ok, err := rs.ledger.SessionOwner(ctx, providerID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session owner: %w", err)
	}
	if !ok {
		return rs.announce(ctx, chatID, providerID, sessionID, "", relayID)
	}
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return nil, err
	}
	held, err := rs.heldRelayID(ctx, owner, providerID, sessionID)
	if err != nil {
		return nil, err
	}
	if c, holds := st.Conversation(providerID); !holds || !c.Resumable() {
		if err := rs.giveUp(ctx, owner, chatID, providerID, sessionID); err != nil {
			return nil, err
		}
	}
	return rs.announce(ctx, chatID, providerID, sessionID, owner, held)
}

func (rs *Runners) heldRelayID(ctx context.Context, owner, providerID, sessionID string) (string, error) {
	st, err := rs.ledger.State(ctx, owner)
	if err != nil {
		return "", fmt.Errorf("held relay id: %w", err)
	}
	for _, c := range st.Conversations() {
		if c.Provider == providerID && c.SessionID == sessionID {
			return c.RelayID, nil
		}
	}
	return "", nil
}

func (rs *Runners) giveUp(ctx context.Context, owner, toChat, providerID, sessionID string) error {
	live, ok := rs.live.byChat(owner)
	carrier := ""
	if ok && live.ProviderID == providerID {
		carrier = live.ID
	}
	rows, _, err := rs.ledger.Do(ctx, owner, func(st ledger.State) ([]ledger.Row, error) {
		return ledger.MoveAway(st, providerID, sessionID, toChat, carrier)
	})
	if err != nil {
		return fmt.Errorf("give up the session: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	if carrier != "" {
		rs.endOffLock(ctx, live, domain.AgentExitMoved)
	}
	rs.touch(ctx, owner)
	return nil
}

func (rs *Runners) endOffLock(ctx context.Context, runner engineagents.Runner, cause string) {
	if _, ok := rs.markEnding(runner, cause); !ok {
		return
	}
	rs.background.run(ctx, func(ctx context.Context) {
		if err := rs.endMarked(ctx, runner); err != nil {
			slog.WarnContext(ctx, "agent: end a CLI off its chat's session", "runner_id", runner.ID, "err", err)
		}
	})
}
