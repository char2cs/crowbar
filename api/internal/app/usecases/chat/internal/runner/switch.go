package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

func (rs *Runners) SwitchProvider(
	ctx context.Context,
	chatID string,
	targetProviderID string,
) (string, error) {
	defer rs.spawns.Lock(chatID)()
	return rs.switchProviderLocked(ctx, chatID, targetProviderID)
}

// The caller already holds chatID's spawn gate: SwitchProvider above takes it,
// and ResumeChat reaches this from inside its own. inflight.Gate is not reentrant, so
// wiring either caller to SwitchProvider instead compiles and deadlocks that
// goroutine on its own gate forever.
func (rs *Runners) switchProviderLocked(
	ctx context.Context,
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
		chat, err := rs.chats.GetChat(ctx, chatID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: chat: %w", err)
		}
		// Protect a React replacement that has not emitted its acceptance hook
		// yet. This first check happens before waiting; the interlocked check just
		// before displacement closes the hook-between-checks race.
		if err := rs.requireNoPendingPromptDelivery(ctx, chat); err != nil {
			return "", err
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
		if err := rs.turns.AwaitTurnComplete(ctx, chatID); err != nil {
			return "", err
		}

		priorSessionID, leftAt, err := rs.resumableConversation(ctx, chat, targetProviderID)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: resumable conversation: %w", err)
		}
		resuming := priorSessionID != ""

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
		conversation, err := rs.conversations.AssembleConversation(ctx, chatID, resuming, leftAt)
		if err != nil {
			return "", fmt.Errorf("agent: switch provider: assemble handoff: %w", err)
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

		// Resume arg must be split into separate argv tokens: exec.Command does NOT split
		// a string on whitespace, so a whole "--resume {id}" template handed to a single
		// pass_arg would become one literal argument.
		var resumeSteps []engineagents.InjectStep
		if resuming {
			resumeSteps = nativeResumeSteps(d, priorSessionID)
		}

		// Resume args go first so a positional resume_context_inject — codex's `resume
		// <id>` subcommand, or claude's own --resume value — precedes rather than
		// follows the positional context pointer built from it.
		return rs.spawnRunner(
			ctx, chatID, chat.WorkspaceID, targetProviderID,
			"", resumeSteps, nil, conversation, gapTurns, resuming, priorSessionID, false, "",
		)
	}
}

func (rs *Runners) displaceForSwitch(
	ctx context.Context,
	chat domain.Chat,
) (bool, error) {
	unlockTurnStart := rs.turnStarts.Lock(chat.ID)
	defer unlockTurnStart()

	if err := rs.requireNoPendingPromptDelivery(ctx, chat); err != nil {
		return false, err
	}
	if len(rs.inflightTurns.Inflight(chat.ID)) > 0 {
		// A prompt began after the first wait. Let its hook finish, then rebuild the
		// handoff from the now-newer record before trying again.
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
		return true, nil
	}
	if err := rs.quitOutgoingCLI(ctx, chat.ID); err != nil {
		return false, err
	}
	return false, nil
}

func (rs *Runners) quitOutgoingCLI(
	ctx context.Context,
	chatID string,
) error {
	live, err := rs.runnerStore.LiveRunnerForChat(ctx, chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return nil // dormant: nothing to quit
	}
	if err != nil {
		return fmt.Errorf("agent: switch provider: live runner: %w", err)
	}
	if err := rs.term.TerminateGraceful(ctx, live.TerminalSession); err != nil {
		if !errors.Is(err, engineterminal.ErrSessionNotFound) {
			// The CLI is still on its chat, and it stays there: the switch is aborted with
			// nothing changed rather than half-done.
			return fmt.Errorf("agent: switch provider: terminate outgoing terminal: %w", err)
		}
		slog.WarnContext(ctx, "agent: switch provider: outgoing terminal session already gone before terminate; continuing switch",
			"chat_id", chatID, "runner_id", live.ID, "terminal_session_id", live.TerminalSession, "err", err)
	}
	// An api-transport runner's serve process is NOT the terminal session above —
	// it is a separate background process (apiconn.go's forkServeProcess), never a
	// PTY, for exactly the hotswap:false shape codex declares: no attach at spawn,
	// so no PTY ever exists to take it down on exit. onRunnerExit's own drop only
	// fires from a PTY dying, which never happens here — confirmed live as a
	// process leak: every switch away from codex left its serve process running,
	// and dozens accumulated over one session. Safe to call unconditionally; it is
	// a no-op for the hooks-only common case (claude) and for a codex runner
	// already torn down some other way.
	rs.apiConns.drop(live.ID)
	// A failed displace ABORTS the switch, and this is the one teardown where it must: the
	// caller's very next act is to spawn the incoming CLI, so continuing would place a
	// second runner on a chat the first one is still recorded on — the two-live-CLIs state
	// this whole model exists to make unrepresentable. Aborting is cheap here and costs the
	// user nothing they cannot get back: the outgoing CLI is already dead or dying, so the
	// chat simply drops to dormant when its PTY goes, and Resume revives it.
	if err := rs.displace(ctx, live); err != nil {
		return fmt.Errorf("agent: switch provider: %w", err)
	}
	return nil
}

func (rs *Runners) resumableConversation(
	ctx context.Context,
	chat domain.Chat,
	targetProviderID string,
) (sessionID string, leftAt time.Time, err error) {
	convs, err := rs.runnerStore.ConversationsForChat(ctx, chat.ID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("conversations: %w", err)
	}
	// Oldest first, so the LAST match is the most recent conversation this provider
	// held in this chat.
	for _, c := range convs {
		if c.ProviderID == targetProviderID {
			sessionID = c.SessionID
		}
	}
	if sessionID == "" {
		return "", time.Time{}, nil
	}

	leftAt, found, err := rs.activity.LastTurnForSession(ctx, chat.ID, targetProviderID, sessionID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("last turn for session: %w", err)
	}
	if !found {
		// The CLI reported this conversation id but never recorded a turn under it, so
		// there is no conversation on disk to resume. Spawn fresh.
		slog.InfoContext(ctx, "agent: prior conversation has no recorded turns; spawning fresh instead of resuming",
			"chat_id", chat.ID, "provider", targetProviderID, "session_id", sessionID)
		return "", time.Time{}, nil
	}
	return sessionID, leftAt, nil
}
