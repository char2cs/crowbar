package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
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
	park, release, err := rs.spawns.Acquire(ctx, chatID)
	if err != nil {
		return "", err
	}
	defer release()
	defer rs.enterPhase(ctx, chatID, rs.replacementPhase(ctx, chatID))()
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
		// Protect a React replacement that has not emitted its acceptance hook
		// yet — by WAITING for it, not by refusing it. It is the same state as a
		// turn in flight one moment earlier, and the turn below is waited for.
		// The interlocked check inside displaceForSwitch still refuses outright:
		// that one runs under turnStarts, which the hook that would release it
		// must take.
		if err := rs.awaitPromptDeliverySettled(park, chat); err != nil {
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
			resumeSteps = resumeInjectionSteps(d, priorSessionID)
		}

		// Resume args go first so a positional resume_context_inject — codex's `resume
		// <id>` subcommand, or claude's own --resume value — precedes rather than
		// follows the positional context pointer built from it.
		runnerID, err := rs.spawnRunner(
			ctx, chatID, chat.WorkspaceID, targetProviderID,
			"", resumeSteps, nil, conversation, gapTurns, resuming, priorSessionID, false, "",
		)
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
		_, liveErr := rs.runnerStore.LiveRunnerForChat(ctx, chat.ID)
		switch {
		case liveErr == nil:
			return true, nil
		case !errors.Is(liveErr, agentrunner.ErrNotFound):
			return false, fmt.Errorf("agent: switch provider: work-check live runner: %w", liveErr)
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
	// live.TerminalSession above is the ORIGINAL companion PTY every api-transport
	// spawn forks alongside its connection — never reassigned, so it names a
	// different, LEAKED process once SwitchToTerminal has run: that call forks a
	// THIRD, separate PTY for the native view and tracks it only in rs.attached,
	// exactly the one the user is actually looking at. Switching provider away
	// from a chat mid-attach must take that one down too, and forget it here —
	// the same gap retire() had (lifecycle.go) before its own fix, for the
	// identical reason: SwitchToNative is otherwise the only place that ever
	// clears rs.attached, and a chat switched away from while attached never
	// reaches it. Best-effort, like retire()'s own: the outgoing CLI is already
	// being torn down regardless, so a stuck attached view must not abort a
	// switch that has already committed to happening.
	if view, ok := rs.attached.get(live.ID); ok {
		rs.attached.drop(live.ID)
		if err := rs.term.TerminateGraceful(ctx, view.termSessID); err != nil &&
			!errors.Is(err, engineterminal.ErrSessionNotFound) {
			slog.WarnContext(ctx, "agent: switch provider: terminate attached native view (best-effort, continuing)",
				"runner_id", live.ID, "terminal_session_id", view.termSessID, "err", err)
		}
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

// sessionAnnounceCrashWindow bounds how recently a conversation must have been
// FIRST SEEN for a turnless session to still be read as the announce-then-crash
// race resumableConversation exists to catch, rather than as a conversation
// that simply predates the activity table (see below). A provider that crashes
// before completing its first turn does so within moments of announcing the
// session — it is a startup crash, not a stall — so tens of seconds comfortably
// covers the race without also swallowing conversations that are merely old.
//
// termwait's constants (DefaultStallQuiet and friends) are NOT reused here:
// every one of them bounds how long a LIVE, currently-running CLI may go quiet
// before Crowbar treats it as stuck. This is a different question — how old a
// conversation's FIRST ANNOUNCEMENT is — asked long after any such CLI is gone,
// so borrowing one would just be reaching for a number that happens to exist
// rather than one that means the right thing.
const sessionAnnounceCrashWindow = 30 * time.Second

// sessionLegacyMinAge bounds how old a turnless session's first announcement
// must be before its absence from the activity table is even ELIGIBLE to mean
// "predates this table" — as opposed to "simply abandoned," the ordinary shape
// of switching to a provider and back before ever sending it anything, which
// ages past sessionAnnounceCrashWindow exactly like a genuine crash does. A
// real migration is measured in days at the very least, so a day is a
// deliberately generous floor: anything newer cannot plausibly be legacy data,
// whatever else is true about it.
const sessionLegacyMinAge = 24 * time.Hour

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
	var firstSeenAt time.Time
	for _, c := range convs {
		if c.ProviderID == targetProviderID {
			sessionID = c.SessionID
			firstSeenAt = c.FirstSeenAt
		}
	}
	if sessionID == "" {
		return "", time.Time{}, nil
	}

	leftAt, found, err := rs.activity.LastTurnForSession(ctx, chat.ID, targetProviderID, sessionID)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("last turn for session: %w", err)
	}
	if found {
		return sessionID, leftAt, nil
	}
	// No turn is recorded for this session in the activity table. That alone is
	// ambiguous: it is exactly what a provider that announced a session and then
	// crashed before its first turn also looks like — but it is ALSO exactly what
	// every conversation from before the activity table existed looks like,
	// forever, no matter how much real history it has on the provider's own side
	// (see this function's package-level doc references for the migration this
	// guards against). Age is what tells the two apart — UNLESS this SAME
	// provider already has a recorded turn on THIS chat, under a different
	// session, which age cannot overrule: proof the activity table was live for
	// this exact (chat, provider) pair rules out "predates the table" no matter
	// how old the current session's own first announcement is. Without this, a
	// session that crashed on its first turn and was not retried within the
	// window became permanently unresumable — every later attempt found the same
	// zero rows, aged past the window, and kept re-resuming a corpse that dies
	// again in a second or two.
	//
	// Scoped to THIS provider's own sessions, not a chat-wide turn count: a chat
	// that switched providers has real, table-live history for the OTHER
	// provider long before this one ever ran, and counting turns chat-wide would
	// misread that as proof about a provider it says nothing about — discarding
	// a genuinely resumable, pre-migration session for the provider actually
	// being resumed.
	for _, c := range convs {
		if c.ProviderID != targetProviderID || c.SessionID == sessionID {
			continue
		}
		_, siblingFound, err := rs.activity.LastTurnForSession(ctx, chat.ID, targetProviderID, c.SessionID)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("last turn for sibling session: %w", err)
		}
		if siblingFound {
			slog.InfoContext(ctx, "agent: prior conversation has no recorded turns but this provider has others on this chat; spawning fresh instead of resuming a corpse",
				"chat_id", chat.ID, "provider", targetProviderID, "session_id", sessionID)
			return "", time.Time{}, nil
		}
	}
	if time.Since(firstSeenAt) < sessionAnnounceCrashWindow {
		// Recent enough to be the genuine crash race: the CLI reported this
		// conversation id but never recorded a turn under it, so there is no
		// conversation on disk to resume. Spawn fresh.
		slog.InfoContext(ctx, "agent: prior conversation has no recorded turns; spawning fresh instead of resuming",
			"chat_id", chat.ID, "provider", targetProviderID, "session_id", sessionID)
		return "", time.Time{}, nil
	}
	if time.Since(firstSeenAt) < sessionLegacyMinAge {
		// Old enough to rule out the immediate crash race, but nowhere near old
		// enough to plausibly predate a migration that shipped in the past — no
		// real migration is measured in minutes. This is the ordinary shape of
		// switching to a provider and back before ever sending it anything:
		// the session was announced, then simply abandoned, not crashed and not
		// legacy. sessionAnnounceCrashWindow (30s) only separates "still
		// mid-crash" from "not"; reusing IT for the legacy question mistook
		// every session merely switched away from for a minute or two as
		// decades-old data, sent --resume at a session id the provider itself
		// never wrote a conversation file for, and either failed outright or
		// left the CLI in a broken half-started state.
		slog.InfoContext(ctx, "agent: prior conversation has no recorded turns and is far too recent to be legacy data; spawning fresh instead of resuming an abandoned session",
			"chat_id", chat.ID, "provider", targetProviderID, "session_id", sessionID, "first_seen_at", firstSeenAt)
		return "", time.Time{}, nil
	}
	// Old enough that the missing row means "predates this table", not "crashed
	// before its first turn" and not merely abandoned. The session id is still
	// real and still resumable — refusing it here would be strictly more
	// destructive than the race this guard exists to catch. There is no
	// per-turn record to draw the gap cutoff from, so chat.LastActivityAt
	// (folded from the chat's own turn events, which survive this migration
	// untouched) stands in for it.
	slog.InfoContext(ctx, "agent: prior conversation predates recorded turns; resuming anyway using the chat's last activity as the gap cutoff",
		"chat_id", chat.ID, "provider", targetProviderID, "session_id", sessionID, "first_seen_at", firstSeenAt)
	return sessionID, chat.LastActivityAt, nil
}
