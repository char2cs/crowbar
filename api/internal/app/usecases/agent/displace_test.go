package agent_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/domain"
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

// ---------------------------------------------------------------------------
// Placement onto a chat evicts whoever else is placed there.
// ---------------------------------------------------------------------------

// TestRegression_ResumeIntoAChatHeldByAnotherCLI_EvictsTheChatIncumbent
//
// The reported bug with ONE variable changed, and it needs no race at all.
//
// Eviction used to be keyed only on who holds the CONVERSATION. Nobody looked at who is
// placed on the destination CHAT. So: chat B is being worked by codex (r3), which has not
// announced its conversation yet — or has announced a different one. B's older claude
// conversation sB is still in Crowbar's history AND still in claude's own /resume picker,
// because Crowbar deliberately never deletes a vendor's session file. The user picks sB
// from inside chat A's CLI. Nobody "holds" sB, so nothing is evicted — and chat B ends up
// with TWO live CLIs on it, indefinitely, both appending to its ledger, one of them
// invisible.
//
// Placement onto a chat must evict whoever else is placed there. That is what makes I2 an
// invariant rather than a coincidence.
func TestRegression_ResumeIntoAChatHeldByAnotherCLI_EvictsTheChatIncumbent(t *testing.T) {
	f := newFixture(t)

	// Chat B: claude ran and spoke (so sB is a real, resumable conversation), then the
	// user switched B to codex. Codex (r3) is now working in B and has announced nothing.
	chatB, claudeB := f.spawn(t, "claude")
	f.announce(t, claudeB, "sB")
	turn(t, f, claudeB, "claude", "what claude said in B")
	r3, err := f.usecase.SwitchProvider(f.ctx, chatB, "codex")
	require.NoError(t, err)
	f.wait()
	r3term := f.runner(t, r3).TerminalSession

	require.Equal(t, r3, mustLive(t, f, chatB).ID, "precondition: codex holds chat B")

	// Chat A, elsewhere. Inside its CLI the user /resumes B's old claude conversation.
	_, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	f.announce(t, r1, "sB")

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"I2: a chat is held by ONE CLI — the one that just took it, never two")

	live := mustLive(t, f, chatB)
	assert.Equal(t, r1, live.ID, "the mover holds chat B")

	assert.Contains(t, f.term.terminateRequestIDs(), r3term,
		"the CLI that WAS on chat B must be evicted, not left running invisibly")

	// And the evictee can no longer write into the chat it lost.
	require.NoError(t, f.usecase.IngestHook(f.ctx, r3, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "codex is still talking in B"})))
	f.wait()
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "codex is still talking in B")
}

// ---------------------------------------------------------------------------
// Displacement must not orphan the turn the displaced CLI left open.
// ---------------------------------------------------------------------------

// TestRegression_AbortedSwitchMidTurn_DoesNotLeaveTheChatSpinningForever
//
// reconcileRunnerExit closes a turn a dying CLI left open by looking at
// runner.CurrentChatID — which Displace erases. So a switch that ABORTS after the outgoing
// CLI has been quit (an unknown target provider is the tested, reachable case) used to
// leave the chat with no runner and Working=true FOREVER: the chat row spins, and the
// workspace's whole overlay spins with it, until the user resumes that chat and completes
// another turn.
//
// Closing the turn at displacement time asserts nothing about liveness: once displaced, no
// hook from that runner can ever reach the chat again, so "nobody is working on this chat"
// is simply the last true thing we can say about it.
func TestRegression_AbortedSwitchMidTurn_DoesNotLeaveTheChatSpinningForever(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "a long-running task"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	// The switch quits the outgoing CLI and THEN fails (unknown provider).
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "not-a-real-provider")
	require.Error(t, err)
	f.wait()

	_, err = f.liveRunnerFor(t, chatID)
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "precondition: the chat has no CLI left")

	assert.False(t, f.chat(t, chatID).Working,
		"a chat nothing is running on must not be left spinning forever")
}

// TestSwitchProvider_MidTurn_ClosesTheOutgoingTurn is the same rule on the SUCCESS path:
// the outgoing CLI is killed mid-turn, so its turn is over — nobody will ever send its
// turn_stop. The incoming CLI's own turns are unaffected.
func TestSwitchProvider_MidTurn_ClosesTheOutgoingTurn(t *testing.T) {
	f := newFixture(t)

	chatID, outgoing := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, outgoing, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "working"})))
	require.True(t, f.chat(t, chatID).Working)

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	assert.False(t, f.chat(t, chatID).Working,
		"the killed CLI's turn is over: it will never send a turn_stop")

	// The incoming CLI can still open its own turn, and the outgoing one's belated PTY
	// death must not close it.
	require.NoError(t, f.usecase.IngestHook(f.ctx, incoming, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "the new CLI is working"})))
	require.True(t, f.chat(t, chatID).Working)

	f.term.exit(t, "term-1")
	f.wait()
	assert.True(t, f.chat(t, chatID).Working,
		"the outgoing runner's exit must not close the incoming runner's turn")
}

// ---------------------------------------------------------------------------
// "" means NOWHERE, everywhere.
// ---------------------------------------------------------------------------

// TestRegression_HookFromADisplacedRunner_NeverTouchesTheChatModel
//
// A displaced-but-still-dying CLI is the NORMAL state of every switched-out, evicted and
// purged runner for as long as SIGTERM takes — and its CurrentChatID is "". An unguarded
// handleTurn hands that "" straight to GetChat, whose lazy self-heal REPLAYS THE ENTIRE
// agentchat EVENT LOG on a miss. So every hook from a dying CLI would replay the whole log.
//
// The fault-injected GetChat is the probe: if the chat model is touched at all, the hook
// fails. It must not be touched — "" is nowhere, and nowhere is not looked up.
func TestRegression_HookFromADisplacedRunner_NeverTouchesTheChatModel(t *testing.T) {
	f, cs, _ := newFaultFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID)) // displaces + kills; the CLI still dies slowly
	f.wait()
	require.Empty(t, f.runner(t, runnerID).CurrentChatID, "precondition: the runner is placed nowhere")

	cs.failGetChat = errors.New("boom: the chat model must not be consulted for nowhere")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "a dying CLI's last words"})),
		"a hook from a runner that is placed nowhere must resolve to nowhere, without a lookup")
}

// mustLive reads the chat's live runner, failing the test if the chat is dormant.
func mustLive(t *testing.T, f testFixture, chatID string) domain.AgentRunner {
	t.Helper()
	r, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	return r
}

// ---------------------------------------------------------------------------
// The rule is general: EVERY placement onto a chat evicts whoever else is there.
// ---------------------------------------------------------------------------

// TestRegression_HookMovesOntoAChatMidSpawn_TheSpawnEvictsIt
//
// The rule "placement onto a chat evicts whoever else is placed there" was applied to
// Move and NOT to Start — and the window that opens is not a hairline, it spans an entire
// process fork:
//
//  1. SwitchProvider holds chat B's gate and quits + DISPLACES the outgoing CLI. Chat B now
//     has ZERO placed runners.
//  2. A hook — never gated, and never may be — moves another live CLI (r1) onto B. Nobody
//     is placed on B and nobody holds that conversation, so nothing is evicted. The Move
//     commits.
//  3. The switch resolves its descriptor, renders its tmp dir, FORKS A PROCESS, and Starts
//     the incoming runner onto B.
//
// Chat B ends up holding r1 AND the incoming CLI, both live, both placed, indefinitely —
// and r1 is invisible, because LiveRunnerForChat hands out the newest arrival while r1 goes
// on appending to B's ledger. The invisible-agent state, restored by the fix that was meant
// to end it.
//
// duringFork drives step 2 INSIDE CreateCommand, so the interleaving is exact rather than
// hoped for: no sleeps, no goroutine racing.
func TestRegression_HookMovesOntoAChatMidSpawn_TheSpawnEvictsIt(t *testing.T) {
	f := newFixture(t)

	// Chat B: claude spoke here, so sB is a real conversation — known to Crowbar, and still
	// in claude's own /resume picker.
	chatB, claudeB := f.spawn(t, "claude")
	f.announce(t, claudeB, "sB")
	turn(t, f, claudeB, "claude", "what claude said in B")

	// Another live CLI, elsewhere.
	_, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "sA")

	// While the switch is forking the incoming CLI, r1's user picks sB out of the picker.
	var once sync.Once
	f.term.duringFork = func() {
		once.Do(func() {
			require.NoError(t, f.usecase.IngestHook(f.ctx, r1, "codex", "session_start",
				mustJSON(t, map[string]any{"session_id": "sB"})))
		})
	}

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatB, "codex")
	require.NoError(t, err)
	f.wait()

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"I2: a chat is held by ONE CLI — a spawn must evict whoever moved in while it forked")

	live := mustLive(t, f, chatB)
	assert.Equal(t, incoming, live.ID, "the chat belongs to the CLI that was spawned onto it")

	assert.Contains(t, f.term.terminateRequestIDs(), f.runner(t, r1).TerminalSession,
		"the CLI that moved in mid-spawn must be evicted, not left running invisibly")

	// And it can no longer write into the chat it was evicted from.
	require.NoError(t, f.usecase.IngestHook(f.ctx, r1, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the invisible agent speaks"})))
	f.wait()
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "the invisible agent speaks")
}

// TestRegression_EvictionHealsAnAlreadyBrokenInvariant: incumbentsOf used to read ONE row,
// so if two runners were somehow already placed on a chat it would evict one and leave the
// other — healing nothing. "Everyone else placed here is retired" must be literally what
// the code does, or I2 is a coincidence rather than an invariant.
func TestRegression_EvictionHealsAnAlreadyBrokenInvariant(t *testing.T) {
	f := newFixture(t)

	chatB, claudeB := f.spawn(t, "claude")
	f.announce(t, claudeB, "sB")
	turn(t, f, claudeB, "claude", "what claude said in B")

	// Two other live CLIs, both moved onto chat B by hooks landing while it was vacant (the
	// mid-spawn window, twice over — this is the already-broken state).
	_, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "s1")
	_, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "s2")
	require.NoError(t, f.runnersMove(t, r1, chatB, "s1"))
	require.NoError(t, f.runnersMove(t, r2, chatB, "s2"))
	require.Len(t, f.placedRunnersFor(t, chatB), 3, "precondition: the invariant is already broken")

	// A third CLI resumes B's conversation. Everyone else on B must go.
	_, mover := f.spawn(t, "claude")
	f.announce(t, mover, "sMover")
	f.announce(t, mover, "sB")

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"an eviction must retire EVERY other runner placed on the chat, not just one")
	assert.Equal(t, mover, mustLive(t, f, chatB).ID)
}
