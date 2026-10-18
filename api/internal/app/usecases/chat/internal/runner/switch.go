package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	park, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return "", err
	}
	defer release()
	defer rs.enterPhase(ctx, chatID, rs.replacementPhase(chatID))()
	return rs.switchProviderLocked(ctx, park, chatID, targetProviderID)
}

// The caller already holds chatID's spawn gate: SwitchProvider above takes it,
// and ResumeChat reaches this from inside its own. inflight.Gate is not reentrant, so
// wiring either caller to SwitchProvider instead compiles and deadlocks that
// goroutine on its own gate forever.
//
// park is the gate's park context: the two waits below park on it, so a Stop
// that preempts the gate abandons them with nothing destroyed (ErrStopped).
// Everything after the waits runs on ctx and is bounded, so a preemption never
// leaves a switch half-done.
func (rs *Runners) switchProviderLocked(
	ctx context.Context,
	park context.Context,
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
	// Read BEFORE anything is torn down, purely to know whether this switch is
	// actually a CHANGE — a resolve failure (a chat no provider has ever run on)
	// means "unknown", not "none", so it is treated the same as "no change" and
	// simply skips the marker rather than risk a false-positive divider on a
	// chat's very first spawn.
	previousProviderID, _ := rs.conversations.ChatProviderID(ctx, chatID)
	for {
		chat, err := rs.chats.GetChat(ctx, chatID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: chat: %w", err)
		}
		// A prompt the provider has not acknowledged yet would be lost with the CLI
		// carrying it, so the switch waits until the ledger records its turn or its
		// settle — the same state as a turn in flight one moment earlier.
		if err := rs.awaitNoPendingPrompt(park, chatID); err != nil {
			return "", parkErr(park, err)
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
		// FINISH THE TURN FIRST. The user can click Switch while the agent is mid-answer, and
		// quitting it there costs the answer twice over: the reply in flight is never written,
		// and — because a CLI killed mid-turn never flushes its native transcript at all — the
		// conversation the next `--resume` names does not exist. That is not a theory; it is
		// the "No conversation found with session ID" the user reported (see awaitTurnComplete).
		//
		// It runs BEFORE every read below, so a switch that is parked is holding nothing but its
		// chat's spawn gate: no aggregate read in progress, no db connection, no half-assembled
		// handoff to go stale while it waits. And it runs before the terminate, so the handoff
		// assembled below contains the turn we waited for.
		//
		// Bounded, not open-ended: see awaitTurnOrForce.
		if err := rs.awaitTurnOrForce(ctx, park, chatID); err != nil {
			return "", parkErr(park, err)
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

		retry, err := rs.displaceForSwitch(ctx, chat)
		if err != nil {
			return "", err
		}
		if retry {
			continue
		}

		runnerID, err := rs.spawnRunner(ctx, chatID, chat.WorkspaceID, targetProviderID, spawnInput{
			launch: &launch, conversation: conversation, gapTurns: gapTurns, resuming: resuming,
		}, false)
		if err != nil {
			return "", err
		}
		// Best-effort, after the switch has actually committed: a failed switch
		// changed nothing, and this is Crowbar's own doing, never something to fail
		// the switch itself over. Only recorded when the provider actually changed
		// — resuming into the same provider it was already on is not a switch.
		//
		// An UNKNOWN previous provider is recorded too, and used not to be. That
		// exemption is what made a real conversion invisible: the chats it fired
		// for were exactly the ones nothing else could name either, so the
		// conversion left no divider in the transcript and no interruption for
		// the next resolver to read. It no longer risks a divider on a chat's
		// first spawn, because no first spawn comes through here — StartRunner
		// and SpawnChat do — and every chat those mint now carries its own
		// provider, so "unknown" means a row minted before that field and nothing
		// else.
		if previousProviderID != targetProviderID {
			if err := rs.turns.RecordChatSwitch(
				ctx, chatID, engineagents.InterruptProviderSwitched, targetProviderID,
			); err != nil {
				slog.WarnContext(ctx, "agent: switch provider: record chat switch (best-effort, continuing)",
					"chat_id", chatID, "err", err)
			}
		}
		return runnerID, nil
	}
}

// parkErr reports a wait abandoned because Stop preempted the gate as
// ErrStopped, and passes every other failure through.
func parkErr(park context.Context, err error) error {
	if inflight.Preempted(park) {
		return ErrStopped
	}
	return err
}

func (rs *Runners) displaceForSwitch(
	ctx context.Context,
	chat domain.Chat,
) (bool, error) {
	unlockTurnStart := rs.turnStarts.Lock(chat.ID)
	defer unlockTurnStart()

	st, err := rs.settleOrphanedPrompts(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: switch provider: %w", err)
	}
	if len(st.PendingPrompts()) > 0 || len(rs.inflightTurns.Inflight(chat.ID)) > 0 {
		// A prompt was steered or began after the waits. Wait for it, then rebuild
		// the handoff from the now-newer record before trying again.
		return true, nil
	}
	working, err := rs.turns.ChatWorking(ctx, chat.ID)
	if err != nil {
		return false, fmt.Errorf("agent: switch provider: final chat work check: %w", err)
	}
	if working {
		// A turn_stop may have handed work to the background after the first await
		// released its runner-scoped turn. Keep the outgoing TUI alive until a later
		// hook authoritatively restates the async-work level as zero.
		//
		// ONLY WHILE THERE IS ONE TO KEEP ALIVE. On a DORMANT chat this wait is
		// unsatisfiable by construction: there is no CLI left to finish the work and
		// none to send the hook that would restate it, so the caller's `continue`
		// spins forever — roughly one lap per awaitTurnOrForce deadline, holding this
		// chat's spawn gate the entire time. That gate is a plain mutex with no
		// context on it, so every later resume, prompt and switch on the chat queues
		// behind the loop and never answers at all: no response, and no access-log
		// line either, because the log is written on completion.
		//
		// ResumeChat enters here for exactly this shape — a dormant chat whose
		// durable `working` outlived the CLI that set it (a SIGKILL mid-background
		// work sends no final stop; see closeAbandonedTurn). So the stale flag
		// stranded the one call whose whole job is to bring that chat back, and the
		// pane that called it sat on its "Resuming this chat…" spinner until the user
		// abandoned the chat. A dormant chat's stale flag is not a reason to wait; it
		// is the thing the resume is here to clear.
		if _, live := rs.live.byChat(chat.ID); live {
			return true, nil
		}
		slog.WarnContext(ctx,
			"agent: switch provider: dormant chat still flagged working; proceeding rather than waiting for a hook that cannot arrive",
			"chat_id", chat.ID)
	}
	if err := rs.quitOutgoingCLI(ctx, chat.ID); err != nil {
		return false, err
	}
	return false, nil
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
