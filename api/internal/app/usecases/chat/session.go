package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (u *runnerUsecase) handleSessionStart(
	ctx context.Context,
	runner engineagents.Runner,
	ev engineagents.CanonicalEvent,
) error {
	if ev.SessionID == "" {
		return nil
	}

	// "Is this conversation one we know?" is answered from APPEND-ONLY history, so it
	// keeps answering long after the runner that opened the conversation has died —
	// which is what makes a /resume into a dormant chat recognisable instead of
	// looking brand new. It replaces the in-memory sessionToChat map AND the boot
	// reseed that used to repopulate it.
	knownChatID, err := u.runners.ChatForSession(ctx, runner.WorkspaceID, ev.SessionID)
	known := err == nil
	if err != nil && !errors.Is(err, agentrunner.ErrNotFound) {
		return fmt.Errorf("agent: ingest hook: lookup session: %w", err)
	}

	switch d := engineagents.Decide(runner.CurrentSession, ev.SessionID, knownChatID, known); d.Kind {
	case engineagents.MoveNoop:
		return nil
	case engineagents.MoveBind:
		// The conversation is recorded AGAINST THE CHAT the runner is on, so that chat had
		// better still be there. It might not be: PurgeChat kills the CLI but a SIGTERM is
		// not synchronous, so a chat deleted seconds after it was created ("wrong provider,
		// undo") can be gone before its CLI has even announced. Binding anyway would write a
		// conversation-history row against a hard-deleted chat — a dangling /resume target,
		// the exact live trap PurgeChat drops that history to prevent.
		ok, err := u.requirePlacement(ctx, runner, runner.CurrentChatID)
		if err != nil || !ok {
			return err
		}
		if _, err := u.runners.BindSession(ctx, runner.ID, ev.SessionID, known, time.Now()); err != nil {
			return fmt.Errorf("agent: ingest hook: bind session: %w", err)
		}
		// A bind takes a conversation, so it obeys I3 exactly as a move does: whoever else is
		// live on that conversation is evicted. On every legitimate path this is a NO-OP, and
		// it is worth knowing why — a CLI's first conversation is normally one Crowbar itself
		// chose, either brand new (an id nobody can be holding) or a resume taken under the
		// chat's spawn gate, having already quit whoever was there.
		//
		// It is a guard rather than a comment because that argument is breakable FROM A
		// CONFIG FILE. ResolveDescriptor merges on-disk descriptor overrides out of crowbar
		// home, and spawn.args is the user's to edit: an override adding `--continue`, or any
		// provider that auto-restores its last session, makes a freshly spawned CLI announce
		// an id CROWBAR NEVER CHOSE — possibly one another live runner is holding. Without
		// this, two CLIs would then write one provider session file and corrupt the
		// PROVIDER'S OWN DATA, with no Go error and no log line. Provider knowledge lives in
		// YAML precisely so that Go never has to trust it; an invariant that depends on what
		// the YAML says is not an invariant.
		u.evictHolderOf(ctx, runner, ev.SessionID)
		return nil
	case engineagents.MoveToNew:
		return u.moveToNewChat(ctx, runner, ev.SessionID)
	case engineagents.MoveToKnown:
		// The destination is resolved from append-only history, which can outlive the chat
		// it names: PurgeChat's history drop is best-effort, and deleting a Crowbar chat
		// deliberately does NOT delete the vendor's session file — so a purged chat's
		// conversation can still be sitting in the CLI's own /resume picker. Pick it, and
		// an unguarded move would repoint the runner at a chat that does not exist: an
		// invisible CLI, writing nowhere, forever.
		ok, err := u.requirePlacement(ctx, runner, d.ChatID)
		if err != nil || !ok {
			return err
		}
		return u.moveToKnownChat(ctx, runner, d.ChatID, ev.SessionID)
	}
	return nil
}

func (u *runnerUsecase) moveToNewChat(
	ctx context.Context,
	runner engineagents.Runner,
	sessionID string,
) error {
	newChatID := uuid.NewString()
	created, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:          newChatID,
		WorkspaceID: runner.WorkspaceID,
		Now:         time.Now(),
	})
	if err != nil {
		return fmt.Errorf("agent: ingest hook: mint chat: %w", err)
	}
	u.work.set(newChatID, created.Working)
	if _, err := u.runners.Move(ctx, runner.ID, newChatID, sessionID, false, time.Now()); err != nil {
		return fmt.Errorf("agent: ingest hook: move to new chat: %w", err)
	}
	// This is the third placement site, and the ONLY one that evicts nobody. That is not an
	// omission: the destination is a uuid minted three lines up, so no runner can be on it and
	// no runner can be holding a conversation nobody has ever announced before. Adding the
	// reads here would put two queries on the hot /clear path to discover a fact we already
	// know for certain. (The other two — Start and moveToKnownChat — land on chats and
	// conversations that CAN be occupied, and both retire whoever is there.)
	//
	// A turn the runner had open on the chat it just LEFT ends here, wherever it had got to:
	// its turn_stop will land on the NEW chat, so nothing will ever close it on the old one.
	// Releasing it is what stops a switch on the old chat waiting for an answer that is now
	// being written somewhere else.
	u.turns.complete(runner.ID)
	// And the turn must be closed on the chat itself, not just released in memory. Left open
	// it is durable: the chat reads Working forever, and the workspace's derived overlay
	// keeps it in the mid-turn set for the life of the daemon — a sidebar spinner running
	// over a workspace where nothing is happening. Same reasoning as the displace path, and
	// the same guards decide it (see closeAbandonedTurn): the runner has gone, and if a
	// successor has already taken the chat then the turn is not ours to close.
	u.closeAbandonedTurn(ctx, runner.CurrentChatID)
	return nil
}

func (u *runnerUsecase) moveToKnownChat(
	ctx context.Context,
	runner engineagents.Runner,
	toChatID string,
	sessionID string,
) error {
	if _, err := u.runners.Move(ctx, runner.ID, toChatID, sessionID, true, time.Now()); err != nil {
		return fmt.Errorf("agent: ingest hook: move to known chat: %w", err)
	}
	// Whatever it was mid-way through on the chat it just left is over there (see
	// moveToNewChat) — released in memory, and closed on the chat so the vacated chat
	// cannot go on advertising a turn whose turn_stop is landing elsewhere.
	u.turns.complete(runner.ID)
	if runner.CurrentChatID != toChatID {
		u.closeAbandonedTurn(ctx, runner.CurrentChatID)
	}

	// Whoever else is live on the CONVERSATION must go (invariant I3).
	u.evictHolderOf(ctx, runner, sessionID)

	// And whoever else is PLACED ON THE CHAT must go too (invariant I2) — a different
	// question from I3, and answering only I3 left the reported bug alive with one variable
	// changed:
	//
	//	Chat B is being worked by codex, which has not announced its conversation yet. B's
	//	older claude conversation is still in Crowbar's history AND still in claude's own
	//	/resume picker (Crowbar never deletes a vendor's session file). Resume it from another
	//	chat's CLI: nobody HOLDS that conversation, so nothing was evicted — and chat B ended
	//	up with two live CLIs on it, indefinitely, both writing its ledger, one invisible.
	//
	// Read AFTER the Move (which is SendWait, so we see ourselves) and retire everyone but
	// us: it is the same rule, and the same call, a Start makes. Both placement paths leave
	// exactly one runner on the chat, which is what makes I2 an invariant rather than a
	// coincidence.
	u.retireOthersOn(ctx, toChatID, runner.ID)
	return nil
}
