package chat_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// TestRegression_DeadPTY_MeansDeadRunner: a runner cannot outlive its PTY. Boot
// reconciliation is the ONE place liveness is reconciled, and it reconciles against the
// single authority — the PTY — rather than against a second opinion Crowbar stored.
//
// What must NOT go with the runner is the chat: killing the live row is what makes the
// chat dormant, and dormant is the state Resume revives from. A reconcile that took the
// conversation history with it would turn a restart into data loss.
func TestRegression_DeadPTY_MeansDeadRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	f.term.dieWithDaemon() // the daemon restarted; every PTY is gone

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	_, err := f.runners.LiveRunnerForChat(f.ctx, chatID)
	require.ErrorIs(t, err, agentrunner.ErrNotFound, "no runner may outlive its PTY")

	// ...but the chat is still resumable: its conversation history is append-only and
	// describes what HAPPENED, which no restart can falsify.
	last, err := f.runners.LastConversation(f.ctx, chatID)
	require.NoError(t, err)
	require.Equal(t, "s1", last.SessionID)
}

// TestRegression_AfterRestart_ResumeStillWorks is the headline regression. The live-runner
// table is durable sqlite and is never truncated at boot, so without this reconcile every
// chat that ever had a runner is UNREVIVABLE for the rest of time: ResumeChat asks
// LiveRunnerForChat first, is handed the stale row of a CLI that died with the daemon, and
// returns it as a no-op. The Resume button silently does nothing, and the pane attaches to
// a dead terminal session.
func TestRegression_AfterRestart_ResumeStillWorks(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	turn(t, f, runnerID, "claude", "the reply the user came back for")

	f.term.dieWithDaemon()

	// The row really does survive the death of the process it describes. That is not a
	// bug in the model — the row is only ever removed by an Exit, and an Exit is only ever
	// emitted because the PTY died, which is a fact nothing was alive to observe.
	stale, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err, "precondition: the pre-restart runner row outlives the daemon")
	require.Equal(t, runnerID, stale.ID)

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	revived, err := f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)

	assert.NotEqual(t, runnerID, revived,
		"Resume must spawn a NEW runner, not hand back the id of a CLI that died with the daemon")
	assert.Equal(t, 2, f.term.callCount(), "Resume must actually launch a vendor CLI")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, revived, live.ID, "and the chat is held by the CLI the user can actually talk to")
}

// TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever: AgentChat.Working is
// reconciled state, never durable truth — a CLI that dies mid-turn never sends the
// turn_stop hook that would close it. When the daemon dies mid-turn there is nobody left
// to run the runtime exit reconcile either, so the chat comes back Working, spins forever,
// and keeps the whole workspace's overlay spinning with it.
//
// Closing that turn asserts nothing about any process: the runner it belonged to is gone
// (we have just Exited it, on the PTY's authority) and no other runner is on the chat, so
// "nobody is working on this chat" is simply the last true thing we can say about it.
func TestRegression_ChatMidTurnAtShutdown_DoesNotSpinForever(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "a long-running task the daemon died in the middle of"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn")

	f.term.dieWithDaemon()

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.False(t, f.chat(t, chatID).Working,
		"a chat that was mid-turn when the daemon died must not spin forever")
}

// TestReconcileRunnersOnBoot_ReapsADisplacedRunnerWhoseKillFailed pins the ONE runner
// nothing else in the system will ever clean up.
//
// Displace takes a runner off its chat while its process is still alive, and the kill that
// follows it is best-effort. When that kill genuinely fails, the runner is left placed
// NOWHERE (empty CurrentChatID), owned by nobody, and never Exited — no hook can reach it,
// no chat points at it, and no teardown path will visit it again. Its row is immortal, and
// the rows accumulate across restarts. Boot reconcile is the only thing that reaps it,
// which is why the reconcile must be driven off ALL live runners rather than off the chats.
func TestReconcileRunnersOnBoot_ReapsADisplacedRunnerWhoseKillFailed(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	require.DirExists(t, tmpDir, "precondition: the spawned CLI has a tmp dir")

	f.term.terminateErr = errors.New("boom: the SIGTERM did not land")

	// A chat delete displaces the runner and then fails to kill it.
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	displaced, err := f.runners.Get(f.ctx, runnerID)
	require.NoError(t, err, "precondition: a displaced runner whose kill failed keeps its live row")
	require.Empty(t, displaced.CurrentChatID, "precondition: Displace erased the chat pointer")

	f.term.dieWithDaemon()

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	_, err = f.runners.Get(f.ctx, runnerID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound,
		"a runner placed nowhere is reaped by boot reconcile or by nothing at all")

	// And its tmp dir goes too — which is only possible because the path is derived from the
	// runner id and provider. Nothing on this row still names the chat it was spawned into,
	// so a chat-keyed path would be unreachable here.
	assert.NoDirExists(t, tmpDir, "the crash-orphan tmp dir of a displaced runner must be reaped")
}

// TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir: on a clean exit the onExit callback
// removes the runner's tmp dir. A crash is precisely the case where that callback never ran,
// so these dirs are the one orphan class that would otherwise accumulate forever, one per
// spawn, across every restart.
//
// They hold the rendered hook config and nothing else — no credentials (the engine has no
// copy_file verb, and no descriptor references one) — so this is hygiene, not a leak.
func TestReconcileRunnersOnBoot_ReapsTheCrashOrphanTmpDir(t *testing.T) {
	f := newFixture(t)

	_, runnerID := f.spawn(t, "claude")
	tmpDir := worktreepath.RunnerDir(f.ws.chatsDir, runnerID, "claude")
	require.DirExists(t, tmpDir, "precondition: the spawned CLI has a tmp dir")

	f.term.dieWithDaemon() // the daemon died: onExit never fired, so the dir was never removed
	require.DirExists(t, tmpDir, "precondition: a crashed daemon reaps nothing on its way out")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	assert.NoDirExists(t, tmpDir, "boot reconcile must reap the tmp dir of a CLI that died with the daemon")
}

// TestReconcileRunnersOnBoot_LeavesALiveRunnerAlone: the reconcile is not a truncation. It
// asks the PTY about every runner and Exits only the ones the PTY says are gone — so a
// runner whose CLI is genuinely still running (the daemon did not restart; something else
// called this) is left exactly where it is, still holding its chat.
func TestReconcileRunnersOnBoot_LeavesALiveRunnerAlone(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "s1")

	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err, "a runner whose PTY is alive is not reaped")
	assert.Equal(t, runnerID, live.ID)
}

// TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer: on an idle machine nothing is running,
// and "nothing is running" is a real answer, not a failure.
func TestReconcileRunnersOnBoot_EmptyIsTheNormalAnswer(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.usecase.ReconcileRunnersOnBoot(f.ctx))
}
