package agent_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
)

// TestOnExit_ActiveSegment_EndsSegmentAndClearsWorking is Task 11 test (a):
// invoking the CreateCommand onExit callback for a still-active segment must
// end that segment and clear the chat's Working flag — the runtime
// process-exit reconcile for a CLI that died without a clean provider switch
// or a hook ever reporting it. StartTurn first so Working is actually true
// going in, so clearing it is a real assertion, not a vacuous one.
func TestOnExit_ActiveSegment_EndsSegmentAndClearsWorking(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.repo.StartTurn(ctx, chatID, timeUnix(1))
	require.NoError(t, err)
	f.wait()

	pre := f.chat(t, chatID)
	require.True(t, pre.Working, "precondition: chat must be Working before the reconcile")
	require.Equal(t, "active", activeSegOf(t, pre, segID).Status)

	require.Equal(t, 1, f.term.callCount())
	onExit := f.term.calls[0].onExit
	require.NotNil(t, onExit)

	onExit()

	post := f.chat(t, chatID)
	assert.False(t, post.Working, "onExit must clear Working for the chat whose live segment just died")
	assert.Empty(t, post.ActiveSegmentID)
	ended := segByID(t, post, segID)
	assert.Equal(t, "ended", ended.Status)
	assert.NotNil(t, ended.EndedAt)
}

// TestOnExit_AlreadyEndedSegment_DoesNotFightSwitch is the double-end / switch
// guard: SwitchProvider already calls EndSegment (and opens a brand-new active
// segment) for the outgoing CLI BEFORE the outgoing PTY's onExit necessarily
// fires (TerminateGraceful can race the reap goroutine). If onExit for the
// now-ended OLD segment ran unconditionally, it would end whatever segment is
// active NOW — the new provider's segment — which must never happen. Firing
// the OLD onExit after the switch has already completed must be a no-op: the
// new segment stays active and Working is untouched.
func TestOnExit_AlreadyEndedSegment_DoesNotFightSwitch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, oldSegID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	require.Equal(t, 1, f.term.callCount())
	oldOnExit := f.term.calls[0].onExit
	require.NotNil(t, oldOnExit)

	newSegID, err := f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()

	_, err = f.repo.StartTurn(ctx, chatID, timeUnix(1))
	require.NoError(t, err)
	f.wait()

	pre := f.chat(t, chatID)
	require.Equal(t, newSegID, pre.ActiveSegmentID, "precondition: the switch's new segment is active")
	require.True(t, pre.Working, "precondition: chat is Working under the NEW segment")

	// The outgoing CLI's belated onExit must not touch the new segment/turn.
	oldOnExit()

	post := f.chat(t, chatID)
	assert.Equal(t, newSegID, post.ActiveSegmentID, "the switch's new segment must still be active")
	assert.Equal(t, "active", activeSegOf(t, post, newSegID).Status)
	assert.True(t, post.Working, "the belated old-segment onExit must not clear Working for the new segment's turn")
	oldSeg := segByID(t, post, oldSegID)
	assert.Equal(t, "ended", oldSeg.Status, "the old segment stays ended (as the switch already left it)")
}

// TestReconcileOnBoot_DeadTerminalSession_EndsSegmentAndStopsTurn is Task 11
// test (b): a chat left Working with an active segment whose terminal session
// is gone (the fake "is-terminal-alive" predicate reports it dead — the
// daemon-crash scenario, since no event records a process death) must be
// reconciled on boot: the segment ends and the turn stops.
func TestReconcileOnBoot_DeadTerminalSession_EndsSegmentAndStopsTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.repo.StartTurn(ctx, chatID, timeUnix(1))
	require.NoError(t, err)
	f.wait()

	require.Equal(t, 1, f.term.callCount())
	seg := activeSegOf(t, f.chat(t, chatID), segID)
	f.term.killSession(seg.TerminalSessionID)

	require.NoError(t, f.usecase.ReconcileOnBoot(ctx))
	f.wait()

	post := f.chat(t, chatID)
	assert.False(t, post.Working, "ReconcileOnBoot must stop the turn for a chat whose active segment's terminal died")
	assert.Empty(t, post.ActiveSegmentID)
	ended := segByID(t, post, segID)
	assert.Equal(t, "ended", ended.Status)
	assert.NotNil(t, ended.EndedAt)
}

// TestReconcileOnBoot_DeadTerminalSession_ReapsCrashOrphanSegmentTmp pins the
// crash-orphan reaper (Finding 1a): a segment that was active when the daemon
// crashed never fired its onExit cleanup, so its per-spawn tmp dir (hook config
// + any codex auth.json copy) is orphaned under the workspace root. Under the
// workspace-root layout there is no global agent-tmp sweep to wipe it, so the
// boot reconcile must remove exactly this segment's dir when it ends the dead
// segment. Spawn creates the dir on disk (real spawnSegment MkdirAll), so its
// presence pre-reconcile is a real precondition, and its absence after is the
// assertion — no timing, we block on the ReconcileOnBoot call itself.
func TestReconcileOnBoot_DeadTerminalSession_ReapsCrashOrphanSegmentTmp(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	tmpDir := worktreepath.SegmentDir(f.ws.chatsDir, chatID, segID, "claude")
	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "precondition: the segment's tmp dir must exist on disk after spawn")

	seg := activeSegOf(t, f.chat(t, chatID), segID)
	f.term.killSession(seg.TerminalSessionID) // the PTY died with the daemon

	require.NoError(t, f.usecase.ReconcileOnBoot(ctx))
	f.wait()

	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err),
		"boot reconcile must reap the crash-orphaned segment's tmp dir")

	// And the segment is still ended by the same reconcile (the reap is additive,
	// not a replacement for the turn-state repair).
	assert.Empty(t, f.chat(t, chatID).ActiveSegmentID)
}

// TestReconcileOnBoot_LiveTerminalSession_KeepsSegmentTmp is the negative twin:
// a chat whose active segment's PTY is still live (restored placeholder) is NOT
// a crash orphan, so its tmp dir must be left in place — reaping it would delete
// a running CLI's live hook config / credentials out from under it.
func TestReconcileOnBoot_LiveTerminalSession_KeepsSegmentTmp(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	tmpDir := worktreepath.SegmentDir(f.ws.chatsDir, chatID, segID, "claude")
	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "precondition: the segment's tmp dir exists after spawn")

	// Session stays alive (no killSession): the segment is not a crash orphan.
	require.NoError(t, f.usecase.ReconcileOnBoot(ctx))
	f.wait()

	_, err = os.Stat(tmpDir)
	assert.NoError(t, err, "a live segment's tmp dir must survive boot reconcile")
}

// TestReconcileOnBoot_WorkingSetByIngestHook_DeadPTY_StopsTurn proves the
// boot-reconcile interrupted-turn branch is now reachable through the PRODUCTION
// path: a real user_prompt hook opens the turn (IngestHook → StartTurn →
// Working), not a direct repo.StartTurn, and a crash then leaves that active
// segment's PTY dead. On boot the segment is ended and the still-open turn is
// stopped — the exact "if chat.Working → StopTurn" branch that was dead code
// while StartTurn/StopTurn had no callers.
func TestReconcileOnBoot_WorkingSetByIngestHook_DeadPTY_StopsTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "do work"})))
	pre := f.chat(t, chatID)
	require.True(t, pre.Working, "precondition: the user_prompt hook must have opened the turn")

	seg := activeSegOf(t, pre, segID)
	f.term.killSession(seg.TerminalSessionID)

	require.NoError(t, f.usecase.ReconcileOnBoot(ctx))
	f.wait()

	post := f.chat(t, chatID)
	assert.False(t, post.Working, "boot reconcile must stop the interrupted turn")
	assert.Empty(t, post.ActiveSegmentID)
	ended := segByID(t, post, segID)
	assert.Equal(t, "ended", ended.Status)
	assert.NotNil(t, ended.EndedAt)
}

// TestReconcileOnBoot_LiveTerminalSession_LeavesChatUntouched: a chat whose
// active segment's terminal session IS still live (per the injected liveness
// predicate — e.g. a session merely reloaded as a suspended placeholder) must
// be left exactly as it was.
func TestReconcileOnBoot_LiveTerminalSession_LeavesChatUntouched(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.repo.StartTurn(ctx, chatID, timeUnix(1))
	require.NoError(t, err)
	f.wait()

	pre := f.chat(t, chatID)
	require.True(t, pre.Working)
	require.Equal(t, segID, pre.ActiveSegmentID)

	// f.term's SessionExists defaults to alive for every session id (nothing
	// killed), so the reconcile must be a total no-op here.
	require.NoError(t, f.usecase.ReconcileOnBoot(ctx))
	f.wait()

	post := f.chat(t, chatID)
	assert.True(t, post.Working, "a chat whose terminal session is still live must not be touched")
	assert.Equal(t, segID, post.ActiveSegmentID)
	assert.Equal(t, "active", activeSegOf(t, post, segID).Status)
}

// TestReconcileOnBoot_NoActiveSegment_IsSkipped: a chat that already has no
// active segment (e.g. it ended cleanly before the crash) is skipped outright
// — there is nothing to check liveness against, and no active segment means
// segmentByID(chat, "") must never match a stray empty-ID row.
func TestReconcileOnBoot_NoActiveSegment_IsSkipped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.repo.EndSegment(ctx, chatID, segID, timeUnix(1))
	require.NoError(t, err)
	f.wait()

	pre := f.chat(t, chatID)
	require.Empty(t, pre.ActiveSegmentID)

	require.NoError(t, f.usecase.ReconcileOnBoot(ctx))
	f.wait()

	post := f.chat(t, chatID)
	assert.Empty(t, post.ActiveSegmentID)
	assert.False(t, post.Working)
}

// TestReconcileOnBoot_ListChatsFailure_ReturnsWrappedError mirrors
// TestSeedRegistry_ListChatsFailure_ReturnsWrappedError's error-path coverage
// for the sibling boot method.
func TestReconcileOnBoot_ListChatsFailure_ReturnsWrappedError(t *testing.T) {
	f, fs := newFaultFixture(t)
	fs.failListChats = fmt.Errorf("boom: list chats")
	ctx := context.Background()

	err := f.usecase.ReconcileOnBoot(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reconcile on boot")
}

// TestReconcileOnBoot_EndSegmentFailure_LogsAndContinuesOtherChats: a
// per-chat EndSegment failure must not abort the whole boot reconcile — the
// next chat must still be reconciled.
func TestReconcileOnBoot_EndSegmentFailure_LogsAndContinuesOtherChats(t *testing.T) {
	f, fs := newFaultFixture(t)
	ctx := context.Background()

	chatA, segA, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	chatB, segB, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	segAObj := activeSegOf(t, f.chat(t, chatA), segA)
	segBObj := activeSegOf(t, f.chat(t, chatB), segB)
	f.term.killSession(segAObj.TerminalSessionID)
	f.term.killSession(segBObj.TerminalSessionID)

	fs.failEndSeg = fmt.Errorf("boom: end segment")

	// EndSegment fails for every chat via the fault-injecting wrapper; the
	// function must still return nil (best-effort) rather than abort.
	require.NoError(t, f.usecase.ReconcileOnBoot(ctx))

	// Both chats are untouched since every EndSegment failed — nothing to
	// assert beyond "ReconcileOnBoot didn't error out and didn't panic."
	f.wait()
	assert.Equal(t, "active", activeSegOf(t, f.chat(t, chatA), segA).Status)
	assert.Equal(t, "active", activeSegOf(t, f.chat(t, chatB), segB).Status)
}
