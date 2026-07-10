package agent_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// TestPurgeChat_TerminatesActiveSegmentPTY_AndForgets pins the standalone
// chat-purge half of Task 5 (the workspace-delete cascade's own PTY teardown +
// Forget lives in repositories.Container.forgetAgentChats, unaffected by
// this): PurgeChat terminates the chat's active segment's live vendor-CLI PTY
// and hard-deletes the chat via asynx Forget. Forget erases the aggregate
// outright, so a genuinely gone chat 404s even a by-id lookup.
func TestPurgeChat_TerminatesActiveSegmentPTY_AndForgets(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	active := activeSegOf(t, f.chat(t, chatID), segID)
	require.NotEmpty(t, active.TerminalSessionID)

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID))

	assert.Contains(t, f.term.terminatedIDs(), active.TerminalSessionID)

	_, err = f.usecase.GetChat(ctx, chatID)
	require.Error(t, err, "a forgotten chat must not be readable even by direct GetChat")
	assert.True(t, errors.Is(err, agentchat.ErrNotFound))

	chats, err := f.usecase.ListChats(ctx)
	require.NoError(t, err)
	for _, c := range chats {
		assert.NotEqual(t, chatID, c.ID, "a forgotten chat must not appear in ListChats")
	}
}

// TestPurgeChat_UnbindsSegmentsFromRegistry is the regression guard for a
// zombie-chat bug found via live daemon testing: a hard-deleted chat reappeared
// in the read model (list + by-id GET) after deletion. Root cause: the PTY
// teardown fires reconcileSegmentExit asynchronously; its GetChat guard races
// asynx's ASYNC (goroutine-per-handler) onForget row-delete, so a fast-exiting
// CLI emits a segment_ended event whose Save re-creates the row after onForget
// deleted it. The order-independent fix is that PurgeChat unbinds the chat's
// segments from the in-memory registry, so reconcileSegmentExit hits its FIRST
// guard — ChatFor(segID) == (,false) — and no-ops before ever touching the racy
// read model. This asserts that unbind: after PurgeChat, the registry no longer
// maps the chat's spawned segment to any chat.
func TestPurgeChat_UnbindsSegmentsFromRegistry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	// Precondition: the spawn bound the segment to its chat.
	got, ok := f.registry.ChatFor(segID)
	require.True(t, ok, "spawn should have bound the segment")
	require.Equal(t, chatID, got)

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID))

	// After purge the segment must be unbound, so the async PTY-teardown
	// reconcile no-ops at ChatFor and cannot resurrect the deleted chat.
	_, ok = f.registry.ChatFor(segID)
	assert.False(t, ok, "PurgeChat must unbind the chat's segments from the registry")
}

// TestPurgeChat_TerminateFailure_SessionAlreadyGone_ContinuesPurge covers the
// purge's tolerance for a terminal session that is already gone (the one error
// the real terminal engine returns today): the purge must still proceed rather
// than get stuck because the CLI process had already exited on its own.
func TestPurgeChat_TerminateFailure_SessionAlreadyGone_ContinuesPurge(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID))

	_, err = f.usecase.GetChat(ctx, chatID)
	assert.True(t, errors.Is(err, agentchat.ErrNotFound))
}

// TestPurgeChat_TerminateFailure_OtherError_IsBestEffort_StillPurges: a
// genuine TerminateGraceful failure (not "session already gone") must NOT
// abort the purge — the active-segment PTY teardown is best-effort.
func TestPurgeChat_TerminateFailure_OtherError_IsBestEffort_StillPurges(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	active := activeSegOf(t, f.chat(t, chatID), segID)
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID), "a genuine terminate failure must not abort the purge (best-effort)")

	assert.Contains(t, f.term.terminateRequestIDs(), active.TerminalSessionID)

	_, err = f.usecase.GetChat(ctx, chatID)
	assert.True(t, errors.Is(err, agentchat.ErrNotFound))
}

// TestPurgeChat_NoActiveSegment_SkipsTerminate_StillPurges: a chat whose
// active segment has already ended (ActiveSegmentID cleared) has nothing to
// terminate, but the purge must still proceed.
func TestPurgeChat_NoActiveSegment_SkipsTerminate_StillPurges(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	_, err = f.repo.EndSegment(ctx, chatID, segID, timeUnix(2))
	require.NoError(t, err)
	f.wait()
	require.Empty(t, f.chat(t, chatID).ActiveSegmentID, "the segment must be ended before this test's assertion is meaningful")

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID))

	assert.Empty(t, f.term.terminatedIDs(), "no active segment means nothing to terminate")
	_, err = f.usecase.GetChat(ctx, chatID)
	assert.True(t, errors.Is(err, agentchat.ErrNotFound))
}

// TestPurgeChat_ReapsChatDirOnDisk pins Important-2: a standalone hard delete must
// remove the chat's PLAINTEXT on-disk footprint (its handoff ledger + any residual
// per-segment tmp dir), not only Forget the aggregate. The spawn creates the chat
// dir on disk (real MkdirAll), so its presence pre-purge is a real precondition and
// its absence after is the assertion — no timing, we block on PurgeChat itself.
func TestPurgeChat_ReapsChatDirOnDisk(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	chatDir := filepath.Join(f.ws.chatsDir, chatID)
	segDir := worktreepath.SegmentDir(f.ws.chatsDir, chatID, segID, "claude")
	_, err = os.Stat(segDir)
	require.NoError(t, err, "precondition: the spawned segment's dir exists under the chat dir")

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID))

	_, err = os.Stat(chatDir)
	assert.True(t, os.IsNotExist(err), "purge must reap the chat's on-disk dir (ledger + segment tmp)")
}

// TestPurgeChat_ReapFailure_StillPurges: the on-disk reap is best-effort — even if
// the chat dir cannot be resolved (workspace-reader error), the aggregate is still
// Forgotten and PurgeChat returns nil rather than failing a delete the user asked
// for. The reader error is armed only AFTER the spawn so the spawn itself succeeds.
func TestPurgeChat_ReapFailure_StillPurges(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.ws.err = errors.New("boom: workspace lookup for reap")

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID), "a reap-path failure must not abort the purge")

	_, err = f.usecase.GetChat(ctx, chatID)
	assert.True(t, errors.Is(err, agentchat.ErrNotFound), "the aggregate is still Forgotten")
}

// TestPurgeChat_ReapRefusesChatsDirOutsideHome pins the Task 7 removal-site
// backstop: if AgentChatsDir ever resolves a chats dir OUTSIDE crowbar home — the
// scenario a crafted repo RemoteSlug containing "../" creates, since filepath.Join
// collapses ".." and can escape home — the hard-delete reap must REFUSE the
// os.RemoveAll rather than delete a path on the user's real filesystem. The
// escaping dir carries a sentinel; the assertion (no timing, os.Stat only) is that
// the sentinel SURVIVES the purge.
func TestPurgeChat_ReapRefusesChatsDirOutsideHome(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Stand in for a chats dir that escaped home (what a "../"-poisoned slug would
	// yield): a directory that is NOT under f.ws.home.
	escaped := t.TempDir()
	f.ws.chatsDir = escaped

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	sentinel := filepath.Join(escaped, chatID, "sentinel")
	require.NoError(t, os.MkdirAll(filepath.Dir(sentinel), 0o755))
	require.NoError(t, os.WriteFile(sentinel, []byte("x"), 0o644))

	require.NoError(t, f.usecase.PurgeChat(ctx, chatID))

	_, statErr := os.Stat(sentinel)
	assert.NoError(t, statErr,
		"a chats dir outside crowbar home must NEVER be removed by the purge reap")
}

// TestPurgeChat_UnknownChat_ReturnsWrappedError: PurgeChat on an id with no
// chat wraps the GetChat lookup failure rather than panicking or silently
// no-oping.
func TestPurgeChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.PurgeChat(ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "purge chat: get")
}
