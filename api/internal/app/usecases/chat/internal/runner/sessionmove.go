// Package runner (file sessionmove.go) moves a session another chat holds
// onto a chat whose own conversation never turned: the holder gives it up
// first, so no two chats ever hold one session.
package runner

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// claimHeld announces sessionID, which another chat holds, on chatID: a
// conflict while chatID holds a conversation, else the session moves here.
func (rs *Runners) claimHeld(ctx context.Context, chatID, providerID, sessionID string) ([]ledger.Row, error) {
	owner, ok, err := rs.ledger.SessionOwner(ctx, providerID, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session owner: %w", err)
	}
	if !ok {
		return rs.announce(ctx, chatID, providerID, sessionID, "") // its holder was erased meanwhile
	}
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if c, holds := st.Conversation(providerID); !holds || !c.Resumable() {
		if err := rs.giveUp(ctx, owner, chatID, providerID, sessionID); err != nil {
			return nil, err
		}
	}
	return rs.announce(ctx, chatID, providerID, sessionID, owner)
}

// giveUp loses sessionID on owner to toChat, and ends owner's runner on it:
// one CLI per session.
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

// endOffLock ends runner for cause without waiting: its exit is recorded under
// its chat's lock, which the caller may hold.
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
