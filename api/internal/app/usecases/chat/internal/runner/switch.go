package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

func (rs *Runners) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: switch provider: %w", err)
	}
	defer h.release()
	defer rs.enterPhase(ctx, chatID, rs.replacementPhase(chatID))()
	return rs.switchProviderLocked(ctx, h, chatID, targetProviderID)
}

// The caller already holds chatID through h: SwitchProvider above takes it,
// and ResumeChat reaches this from inside its own. The chat's lock is not
// reentrant, so wiring either caller to SwitchProvider instead deadlocks.
//
// Every wait (a pending prompt, the outgoing CLI's exit) runs outside the lock
// and parks on h, so a Stop abandons it (ErrStopped); after one, the switch
// decides again from the top.
func (rs *Runners) switchProviderLocked(
	ctx context.Context,
	h *hold,
	chatID string,
	targetProviderID string,
) (string, error) {
	// REFUSE A DISABLED TARGET BEFORE ANYTHING IS TORN DOWN. spawnRunner guards it
	// too, but that guard fires at the END of this function — after the outgoing
	// CLI has already been quit — so a switch that only checked there would leave
	// the chat with no agent at all. ResumeChat enters here, so a dormant chat is
	// held to the same rule as a fresh one.
	if err := rs.providers.RequireProviderEnabled(ctx, targetProviderID); err != nil {
		return "", err
	}
	for {
		if _, _, err := rs.settledRunner(h); err != nil {
			return "", fmt.Errorf("agent: switch provider: %w", err)
		}
		chat, err := rs.chats.GetChat(ctx, chatID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: chat: %w", err)
		}
		// A running turn refuses the switch: quitting the CLI would cost its answer.
		if err := rs.requireNotBusy(ctx, chatID); err != nil {
			return "", err
		}
		// A prompt the provider has not acknowledged yet would be lost with the CLI
		// carrying it, so the switch waits until the ledger records its turn or its
		// settle.
		waited, err := rs.waitOutPendingPrompt(ctx, h, chatID)
		if err != nil {
			return "", err
		}
		if waited {
			continue
		}
		// Resolve the target while the outgoing CLI is still alive. A missing or
		// malformed provider descriptor is a deterministic planning failure, not a
		// reason to destroy the user's current session and leave the chat dormant.
		//
		// Through cwdWorkspaceID, not chat.WorkspaceID: a BUBBLE carries none of
		// its own and runs in its nearest workspace-owning ancestor's worktree
		// (model spec §3.2) — the same fallback spawnPaths, promptTarget, Compact
		// and SlashCatalog already take. Promote respawns through here, and a
		// bubble is the only chat it can be called on.
		cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chatID, chat.WorkspaceID)
		if err != nil {
			return "", err
		}
		crowbarHome, _, _, _, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: preflight worktree dir: %w", err)
		}
		d, err := rs.agents.Get(ctx, crowbarHome, targetProviderID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: resolve descriptor: %w", err)
		}
		launch, err := rs.planLaunch(ctx, chatID, d)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: %w", err)
		}
		resuming := launch.resumeID != ""
		leftAt, err := rs.leftAt(ctx, chatID, targetProviderID, launch.resumeID)
		if err != nil {
			return "", err
		}

		// Read-BEFORE-terminate: the ledger is built from hooks and is already on disk, so
		// assembling the handoff never depends on the outgoing CLI still being alive — and
		// doing it FIRST means a failure here aborts the switch with nothing destroyed,
		// rather than leaving the chat with its old CLI killed and the new one spawned with
		// an EMPTY handoff.
		//
		// A provider resumed into its OWN conversation already holds every turn up to the
		// moment it was switched out, so it is handed only the gap. Replaying the whole
		// ledger to it would duplicate its own history back at it — noise that dilutes the
		// very turns it is meant to notice. A provider new to this chat has no history at
		// all, so it gets the conversation so far — capped to the most recent turns, same
		// as the gap, so a long-running chat does not grow the handoff without bound.
		conversation := ""
		if !launch.lost {
			conversation, err = rs.conversations.AssembleConversation(ctx, chatID, resuming, leftAt)
			if err != nil {
				return "", fmt.Errorf("agent: switch provider: assemble handoff: %w", err)
			}
		}

		// How much of the record is NEW to the provider being resumed. It is what
		// the pointer message uses to ask for exactly the gap rather than for the
		// whole conversation this CLI was already handed once.
		gapTurns := 0
		if resuming {
			gap, gapErr := rs.activity.TurnsSince(ctx, chatID, leftAt)
			if gapErr != nil {
				return "", fmt.Errorf("agent: switch provider: measure handoff gap: %w", gapErr)
			}
			gapTurns = len(gap)
		}

		retry, err := rs.displaceForSwitch(ctx, h, chat)
		if err != nil {
			return "", err
		}
		if retry {
			continue
		}

		return rs.spawnRunner(ctx, chatID, chat.WorkspaceID, targetProviderID, spawnInput{
			launch: &launch, conversation: conversation, gapTurns: gapTurns, resuming: resuming,
		}, false)
	}
}

// displaceForSwitch marks the outgoing CLI ending once the chat is idle and
// quits it outside the chat's lock, then asks the caller to decide again; with
// no CLI on the chat it reports nothing to wait for.
func (rs *Runners) displaceForSwitch(
	ctx context.Context,
	h *hold,
	chat domain.Chat,
) (bool, error) {
	if _, live := rs.live.byChat(chat.ID); !live {
		return false, nil
	}
	if retry, err := rs.endIdleOutgoing(ctx, chat); retry || err != nil {
		return retry, err
	}
	err := h.outside(func(context.Context) error { return rs.quitOutgoingCLI(ctx, chat.ID) })
	return err == nil, err
}

// waitOutPendingPrompt waits, outside the chat's lock, while chatID holds a
// prompt its provider has yet to acknowledge (once those whose carrier left the
// chat are settled); waited says the caller must decide again.
func (rs *Runners) waitOutPendingPrompt(ctx context.Context, h *hold, chatID string) (waited bool, err error) {
	st, err := rs.settleOrphanedPrompts(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("agent: switch provider: %w", err)
	}
	if len(st.PendingPrompts()) == 0 {
		return false, nil
	}
	return true, h.outside(func(park context.Context) error { return rs.awaitNoPendingPrompt(park, chatID) })
}

// endIdleOutgoing is the switch's final idle check, in the outgoing runner's
// turn order. Once it passes the runner is marked ending, so no turn opens on
// it before the quit.
func (rs *Runners) endIdleOutgoing(ctx context.Context, chat domain.Chat) (retry bool, err error) {
	rs.live.inTurnOrder(chat.ID, func() {
		if retry, err = rs.outgoingBusy(ctx, chat); !retry && err == nil {
			rs.markOutgoingEnding(chat.ID)
		}
	})
	return retry, err
}

// outgoingBusy reports whether a prompt pending since the first checks must
// be waited for, and refuses a turn that began since.
func (rs *Runners) outgoingBusy(
	ctx context.Context,
	chat domain.Chat,
) (bool, error) {
	st, err := rs.settleOrphanedPrompts(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: switch provider: %w", err)
	}
	if len(st.PendingPrompts()) > 0 {
		return true, nil
	}
	if st.Busy() {
		return false, ErrPromptBusy
	}
	return false, nil
}

// WaitingForPendingPromptLog is logged the instant a switch parks on a prompt
// the provider has not acknowledged, so a test can block on a real signal.
const WaitingForPendingPromptLog = "agent: switch provider: waiting for the pending prompt"

// awaitNoPendingPrompt parks until the chat holds no prompt the provider has
// yet to acknowledge: quitting the CLI carrying one would lose it. Only the
// ledger's next commit to the chat, or the end of park, releases it.
func (rs *Runners) awaitNoPendingPrompt(park context.Context, chatID string) error {
	logged := false
	for {
		if _, err := rs.settleOrphanedPrompts(park, chatID); err != nil {
			return fmt.Errorf("agent: switch provider: %w", err)
		}
		st, changed, err := rs.ledger.Watch(park, chatID)
		if err != nil {
			return fmt.Errorf("agent: switch provider: watch ledger: %w", err)
		}
		if len(st.PendingPrompts()) == 0 {
			return nil
		}
		if !logged {
			slog.InfoContext(park, WaitingForPendingPromptLog, "chat_id", chatID)
			logged = true
		}
		select {
		case <-changed:
		case <-park.Done():
			return fmt.Errorf("agent: switch provider: waiting for the pending prompt: %w", park.Err())
		}
	}
}

// leftAt is where the gap handed to a resumed sessionID starts: its last turn
// in the activity record the gap is rendered from, so the cut and the gap share
// one clock. Zero, the whole record, when nothing is resumed.
func (rs *Runners) leftAt(ctx context.Context, chatID, providerID, sessionID string) (time.Time, error) {
	if sessionID == "" {
		return time.Time{}, nil
	}
	at, _, err := rs.activity.LastTurnForSession(ctx, chatID, providerID, sessionID)
	if err != nil {
		return time.Time{}, fmt.Errorf("agent: last turn on the session: %w", err)
	}
	return at, nil
}
