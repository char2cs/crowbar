package agent_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
)

// ---------------------------------------------------------------------------
// A runner must never be left pointed at a chat that no longer exists.
// ---------------------------------------------------------------------------

// TestRegression_SessionStartAfterItsChatWasDeleted_IsDroppedNotRecorded
//
// PurgeChat SIGTERMs the chat's CLI, but a SIGTERM is not synchronous — the CLI is
// still alive for a moment, and "delete the chat I just made with the wrong provider"
// is an ordinary thing to do seconds after spawning. If that CLI's first session_start
// lands in the window, an unguarded reducer BINDS it: the conversation projection then
// writes a (deletedChatID, sessionID) row AFTER the purge already dropped that chat's
// history. That row is a live trap — a later /resume of the id resolves to a chat that
// does not exist.
//
// The announcement must be DROPPED, and the CLI killed again: it has nowhere to write.
func TestRegression_SessionStartAfterItsChatWasDeleted_IsDroppedNotRecorded(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	termSess := f.runner(t, runnerID).TerminalSession

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	// The CLI is still alive (SIGTERM has not landed yet) and announces its conversation.
	f.announce(t, runnerID, "s-late")

	_, err := f.runners.ChatForSession(f.ctx, "ws1", "s-late")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound,
		"a conversation must never be recorded against a chat that has been deleted")

	assert.Contains(t, f.term.terminateRequestIDs(), termSess,
		"a CLI with nowhere left to write must not be left running")
}

// TestRegression_ResumeIntoAPurgedChat_DoesNotStrandTheRunner
//
// No race needed for this one. Dropping a chat's conversation history is best-effort in
// PurgeChat (a failure is logged and the delete still succeeds), and deleting a Crowbar
// chat deliberately does NOT delete the vendor's own session file — so that conversation
// is still sitting in claude's /resume picker. Pick it, and ChatForSession resolves to
// the purged chat: the runner would be Moved onto a chat that does not exist, and every
// turn it produced from then on would be silently dropped forever. An invisible CLI.
//
// The move must be refused — it is not a move Crowbar can honour — and the CLI retired.
//
// Note what "refused" does NOT mean: it does not mean the runner stays on chat B. The CLI
// really has left sB; that is a fait accompli we cannot undo (spec §3), so recording it as
// still on B would be a lie, and every turn it took in sA would be filed into B's ledger.
// It has followed a conversation somewhere Crowbar cannot follow it, so it is placed
// NOWHERE and killed. Chat B goes dormant — intact, with its history, and resumable.
func TestRegression_ResumeIntoAPurgedChat_DoesNotStrandTheRunner(t *testing.T) {
	f, _, rs := newFaultFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	turn(t, f, r1, "claude", "content, so sA is a real conversation on disk")

	// The history drop fails: sA keeps resolving to chatA, which is about to be erased.
	rs.failForgetChat = errors.New("boom: the history drop failed")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatA))
	require.Equal(t, chatA, f.chatForSession(t, "sA"),
		"precondition: the purged chat's conversation survived the failed drop")

	chatB, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "sB")
	r2term := f.runner(t, r2).TerminalSession

	// The user picks sA out of the CLI's own picker.
	f.announce(t, r2, "sA")

	moved := f.runner(t, r2)
	assert.NotEqual(t, chatA, moved.CurrentChatID,
		"a runner must never be moved onto a chat that does not exist")
	assert.Empty(t, moved.CurrentChatID,
		"it followed a conversation into nowhere: it is placed nowhere")
	assert.Contains(t, f.term.terminateRequestIDs(), r2term,
		"and it must not be left running")

	// Its turns go nowhere — in particular, not into the chat it left.
	require.NoError(t, f.usecase.IngestHook(f.ctx, r2, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "said inside the purged conversation"})))
	f.wait()
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "said inside the purged conversation")

	// And chat B is intact: dormant, with its history, and resumable.
	_, err = f.chats.GetChat(f.ctx, chatB)
	require.NoError(t, err)
	last, err := f.runners.LastConversation(f.ctx, chatB)
	require.NoError(t, err)
	assert.Equal(t, "sB", last.SessionID)
}

// ---------------------------------------------------------------------------
// Displacement is a placement FACT, not an inference from timestamps.
// ---------------------------------------------------------------------------

// TestRegression_LateSessionAnnouncement_DoesNotStealTheChatFromTheIncomingCLI
//
// The window a newest-arrival ORDERING cannot close. The outgoing CLI has not announced
// a conversation yet (claude takes ~1s; the user can hit Switch Provider well inside
// that). The incoming CLI starts at t2 and is the rightful holder of the chat. Then the
// outgoing CLI's session_start finally lands at t3 — and t3 > t2, so any "whoever
// arrived last" ordering hands the chat back to the DYING runner, and the pane attaches
// to a corpse. That is the very failure the ordering existed to prevent.
//
// Displacement fixes it at the source: the CLI we are killing is taken OFF the chat when
// we kill it, so its late announcement resolves to no chat at all and is dropped.
func TestRegression_LateSessionAnnouncement_DoesNotStealTheChatFromTheIncomingCLI(t *testing.T) {
	f := newFixture(t)

	chatID, outgoing := f.spawn(t, "claude") // t0: spawned, and it announces NOTHING yet

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex") // t2
	require.NoError(t, err)
	f.wait()

	// t3: the outgoing CLI, still dying, finally reports the conversation it came up in.
	f.announce(t, outgoing, "s-late")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, incoming, live.ID, "the chat belongs to the incoming CLI, never the corpse")
	assert.Equal(t, f.runner(t, incoming).TerminalSession, live.TerminalSession,
		"the pane must attach to the incoming CLI's PTY")

	assert.Len(t, f.placedRunnersFor(t, chatID), 1,
		"I2: at most one runner is pointed at a chat, at every instant")
}

// TestRegression_UnkillableEvictee_CannotWriteIntoTheChatItLost
//
// An eviction whose kill FAILS leaves the evictee alive — and, if all we did was record
// the mover's Move, the evictee would still be POINTED at the chat the mover now owns.
// Its turn hooks would then append into that chat's ledger and toggle its Working state:
// two CLIs writing one conversation, which is the corruption invariant I3 exists to
// prevent.
//
// Taking it off the chat is a fact Crowbar owns (placement), and it says nothing about
// whether the process is alive — so it is recorded even when the kill fails.
func TestRegression_UnkillableEvictee_CannotWriteIntoTheChatItLost(t *testing.T) {
	f := newFixture(t)

	_, mover := f.spawn(t, "claude")
	f.announce(t, mover, "sA")
	chatB, incumbent := f.spawn(t, "codex")
	f.announce(t, incumbent, "sB")

	f.term.terminateErr = errors.New("boom: the incumbent refuses to die")

	f.announce(t, mover, "sB") // the mover takes sB; the incumbent is evicted but survives

	live, err := f.liveRunnerFor(t, chatB)
	require.NoError(t, err)
	require.Equal(t, mover, live.ID, "precondition: the mover holds chat B")

	// The unkillable evictee keeps working and finishes a turn.
	require.NoError(t, f.usecase.IngestHook(f.ctx, incumbent, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the evictee is still talking"})))
	f.wait()

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "the evictee is still talking",
		"an evicted CLI must never write into the chat it lost")
	assert.False(t, f.chat(t, chatB).Working,
		"nor toggle its turn state")

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"I3/I2: the evictee is no longer placed anywhere, even though it is still alive")
}

// TestRegression_ConcurrentSwitches_LeaveExactlyOneCLIOnTheChat
//
// Displacement makes the READ side honest. Only serialisation fixes the WRITE side: two
// concurrent SwitchProvider calls both read the same live runner, both kill it, and both
// spawn — leaving two CLIs pointed at one chat, which asynx OCC cannot prevent because
// the two spawns create DIFFERENT aggregates.
//
// The per-chat gate is not the old OpenSegment.Validate guard coming back: it rejects
// nothing, and it never runs on the hook path (a hook must never block or fail). It just
// serialises process creation on a chat, so the second switch sees the first one's runner
// and quits it, exactly as a sequential pair of clicks would.
func TestRegression_ConcurrentSwitches_LeaveExactlyOneCLIOnTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	var g errgroup.Group
	for _, target := range []string{"codex", "claude"} {
		g.Go(func() error {
			_, err := f.usecase.SwitchProvider(f.ctx, chatID, target)
			return err
		})
	}
	require.NoError(t, g.Wait())
	f.wait()

	placed := f.placedRunnersFor(t, chatID)
	assert.Len(t, placed, 1, "two concurrent switches must not leave two CLIs on one chat")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, placed[0].ID, live.ID)
}
