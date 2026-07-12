package agent_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TestRegression_DeleteChat_KillsItsRunner
//
// A runner whose chat no longer exists is a runner with nowhere to write. The chat
// delete cascade must therefore get rid of it — but NOT by deleting its row from the
// read model. That would be Crowbar asserting a liveness fact it does not own (the
// process would still be running, invisible), which is the exact dual-authority
// mistake this model deletes. It kills the PROCESS and lets the row follow.
func TestRegression_DeleteChat_KillsItsRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	r := f.runner(t, runnerID)
	require.NotEmpty(t, r.TerminalSession)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Contains(t, f.term.terminatedIDs(), r.TerminalSession,
		"the chat's CLI must be terminated, not orphaned")

	// The PTY dies, and THAT is what removes the runner.
	f.term.exit(t, r.TerminalSession)
	f.wait()

	_, err := f.runners.Get(f.ctx, runnerID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "no runner may point at a hard-deleted chat")
	_, err = f.runners.LiveRunnerForChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// TestRegression_DeleteChatMidTurn_DoesNotResurrectTheChat is the zombie-chat guard.
//
// Found in live daemon testing: a hard-deleted chat reappeared in the read model
// (list AND by-id GET) after deletion. The PTY teardown fires the runner-exit
// reconcile ASYNCHRONOUSLY, and that reconcile writes to the chat (it closes the turn
// the dead CLI left open); asynx projections are async, so a chat command that
// commits before the Forget can have its read-model Save land AFTER Forget's
// row-delete — re-creating the row.
//
// The old code fought this with an in-memory registry unbind the teardown checked
// first. That registry is gone, so the guard is now structural: the chat is Forgotten
// BEFORE the CLI is killed, and a Forgotten aggregate's event log is erased, so every
// later chat command fails Validate and emits nothing at all. Deleting mid-turn — the
// case that actually writes on exit — must leave the chat gone and staying gone.
func TestRegression_DeleteChatMidTurn_DoesNotResurrectTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "working on it"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn when it is deleted")

	termSess := f.runner(t, runnerID).TerminalSession
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	// ...and now the CLI actually dies, off the reap goroutine, as it does in production.
	f.term.exit(t, termSess)
	f.wait()

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "a deleted chat must not come back as a zombie row")

	chats, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	for _, c := range chats {
		assert.NotEqual(t, chatID, c.ID, "a forgotten chat must not reappear in ListChats")
	}
}

// TestPurgeChat_ForgetsTheChatOutright: Forget erases the aggregate — event log and
// read-model row — so a genuinely gone chat 404s even a by-id lookup.
func TestPurgeChat_ForgetsTheChatOutright(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, err := f.usecase.GetChat(f.ctx, chatID)
	require.Error(t, err, "a forgotten chat must not be readable even by direct GetChat")
	assert.ErrorIs(t, err, agentchat.ErrNotFound)

	chats, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	for _, c := range chats {
		assert.NotEqual(t, chatID, c.ID, "a forgotten chat must not appear in ListChats")
	}
}

// TestPurgeChat_DropsConversationHistory: conversation history is append-only and
// outlives the process, so nothing else ever removes it. A conversation left pointing
// at a hard-deleted chat is a live trap — a later /resume of that id would resolve to
// a chat that no longer exists, and the runner would be moved somewhere unreachable.
func TestPurgeChat_DropsConversationHistory(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-1")
	require.Equal(t, chatID, f.chatForSession(t, "sid-1"), "precondition: the conversation is known")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	_, err := f.runners.ChatForSession(f.ctx, "ws1", "sid-1")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound,
		"the deleted chat's conversations must not keep resolving to it")
	_, err = f.runners.LastConversation(f.ctx, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// TestPurgeChat_DormantChat_NothingToTerminate: a chat no runner points at has no CLI
// to kill, and that absence is an answer, not an error.
func TestPurgeChat_DormantChat_NothingToTerminate(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession) // the CLI exits on its own
	f.wait()

	before := len(f.term.terminateRequestIDs())
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestPurgeChat_TerminateFailure_SessionAlreadyGone_ContinuesPurge covers the purge's
// tolerance for a terminal session that is already gone (the one error the real
// terminal engine returns today).
func TestPurgeChat_TerminateFailure_SessionAlreadyGone_ContinuesPurge(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestPurgeChat_TerminateFailure_OtherError_IsBestEffort_StillPurges: a genuine
// TerminateGraceful failure must NOT abort the purge. An orphaned PTY is a far smaller
// harm than a chat the user can never remove.
func TestPurgeChat_TerminateFailure_OtherError_IsBestEffort_StillPurges(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	termSess := f.runner(t, runnerID).TerminalSession
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID),
		"a genuine terminate failure must not abort the purge (best-effort)")

	assert.Contains(t, f.term.terminateRequestIDs(), termSess)
	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestPurgeChat_ReapsChatDirOnDisk: a standalone hard delete must remove the chat's
// PLAINTEXT on-disk footprint (its handoff ledger + any residual per-spawn tmp dir),
// not only Forget the aggregate.
func TestPurgeChat_ReapsChatDirOnDisk(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	chatDir := filepath.Join(f.ws.chatsDir, chatID)
	segDir := worktreepath.SegmentDir(f.ws.chatsDir, chatID, runnerID, "claude")
	_, err := os.Stat(segDir)
	require.NoError(t, err, "precondition: the spawned runner's dir exists under the chat dir")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, err = os.Stat(chatDir)
	assert.True(t, os.IsNotExist(err), "purge must reap the chat's on-disk dir (ledger + tmp)")
}

// TestPurgeChat_ReapFailure_StillPurges: the on-disk reap is best-effort — even if the
// chat dir cannot be resolved, the aggregate is still Forgotten and PurgeChat returns
// nil rather than failing a delete the user asked for.
func TestPurgeChat_ReapFailure_StillPurges(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.ws.err = errors.New("boom: workspace lookup for reap")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID), "a reap-path failure must not abort the purge")

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "the aggregate is still Forgotten")
}

// TestPurgeChat_ReapRefusesChatsDirOutsideHome pins the removal-site backstop: if
// AgentChatsDir ever resolves a chats dir OUTSIDE crowbar home — the scenario a crafted
// repo RemoteSlug containing "../" creates, since filepath.Join collapses ".." and can
// escape home — the hard-delete reap must REFUSE the os.RemoveAll rather than delete a
// path on the user's real filesystem.
func TestPurgeChat_ReapRefusesChatsDirOutsideHome(t *testing.T) {
	f := newFixture(t)

	// Stand in for a chats dir that escaped home (what a "../"-poisoned slug would
	// yield): a directory that is NOT under f.ws.home.
	escaped := t.TempDir()
	f.ws.chatsDir = escaped

	chatID, _ := f.spawn(t, "claude")

	sentinel := filepath.Join(escaped, chatID, "sentinel")
	require.NoError(t, os.MkdirAll(filepath.Dir(sentinel), 0o755))
	require.NoError(t, os.WriteFile(sentinel, []byte("x"), 0o600))

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, statErr := os.Stat(sentinel)
	assert.NoError(t, statErr,
		"a chats dir outside crowbar home must NEVER be removed by the purge reap")
}

// TestPurgeChat_UnknownChat_ReturnsWrappedError: PurgeChat on an id with no chat wraps
// the lookup failure rather than panicking or silently no-oping.
func TestPurgeChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.PurgeChat(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "purge chat: get")
}
