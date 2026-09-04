//go:build integration

package tests

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLive_GitSnapshotThenLiveUpdate proves the watcher chain on the co-located
// Git topic: the snapshot arrives on connect, then a real on-disk edit
// (re-written on a ticker so the test never races the lazily started watcher
// goroutine) fans out as a live GitStatus frame carrying the dirty file.
func TestLive_GitSnapshotThenLiveUpdate(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	conn := h.dial(wsBase(imported) + "/git/status")

	snapshot := readUntil(t, conn, func(m map[string]any) bool {
		_, ok := m["branch"]
		return ok
	})
	assert.Equal(t, "main", snapshot["branch"])

	// Edit the WATCHED workspace's managed worktree (not the detached repo home).
	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, imported), "README.md", "snapshot then live\n")
	defer stop()

	live := readUntil(t, conn, func(m map[string]any) bool {
		files, ok := m["files"].([]any)
		return ok && len(files) > 0
	})
	files, _ := live["files"].([]any)
	assert.NotEmpty(t, files, "a live GitStatus frame with the dirty file must arrive")
}

// TestLive_FileChangeEvent proves the watcher chain on the co-located Files
// topic: a real edit fans out as a FileChangeEvent scoped to the subscribing
// workspace.
func TestLive_FileChangeEvent(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	conn := h.dial(wsBase(imported) + "/files/ws")

	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, imported), "watched.txt", "edit\n")
	defer stop()

	got := readUntil(t, conn, func(m map[string]any) bool {
		_, ok := m["path"]
		return ok
	})
	assert.Equal(t, imported.workspaceID, got["wsId"])
}

// The standalone repo-scoped Workspaces topic this pinned snapshot-on-subscribe
// for is deleted outright (spec §8 step 6). Its replacement, the repo-scoped
// chat feed, is a live event stream with a deliberately nil Snapshot
// (agentChatDef) — TestLive_WorkspaceSnapshotOnConnect asserted exactly the
// capability that was removed, so it is deleted rather than ported.

// TestLive_GitWsIdScopingIsolatesWorkspaces proves workspace scoping: a Git
// subscriber for workspace A receives A's snapshot but never workspace B's, even
// after B is edited on disk. Two real repos are imported so both watchers are
// driveable. Scoping is now implicit in the hierarchical path (flat wsId
// namespace), not a query param.
func TestLive_GitWsIdScopingIsolatesWorkspaces(t *testing.T) {
	h := newHarness(t)
	a := importProject(t, h)
	b := importProject(t, h)

	conn := h.dial(wsBase(a) + "/git/status")

	// Drain A's snapshot so the subsequent frames are live events only. A starts
	// clean, which is what makes the content discriminator below valid: B dirties
	// README.md and only README.md, A dirties the sentinel and only the sentinel.
	// Content is the ONLY discriminator available, because the Git broadcaster
	// filters on wsId but serialises a BARE GitStatus — there is no wsId on the
	// wire to assert on.
	snapshot := readUntil(t, conn, func(m map[string]any) bool {
		_, ok := m["branch"]
		return ok
	})
	require.False(t, statusHasFile(snapshot, "README.md"),
		"A must start clean, or a README.md frame could not be attributed to B")

	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, b), "README.md", "edit B\n")
	defer stop()

	// Subscribing to B's watcher makes B's events real: this read returns only
	// once B has ACTUALLY broadcast a dirty status, so B is provably live — and
	// its ticker keeps it broadcasting for the remainder of the test.
	bConn := h.dial(wsBase(b) + "/git/status")
	readUntil(t, bConn, func(m map[string]any) bool {
		return statusHasFile(m, "README.md")
	})

	// The negative — "A never receives B's events" — cannot be proven by a read
	// deadline. A deadline shows only "nothing arrived within 1s", which is a
	// guess: it can false-PASS if B is merely slow, and false-FAIL under load.
	// Prove it with a SENTINEL on A's OWN topic instead: dirty a file in A's
	// worktree and block (no deadline) until A's frame carrying it arrives.
	//
	// Throughout that blocking read B is still broadcasting a dirty frame every
	// tick. Were the wsId filter leaking, those frames would be landing on THIS
	// connection the whole time, so we would read one long before A's sentinel
	// could show up. Reaching the sentinel having seen no foreign file therefore
	// proves the isolation exactly, with no clock: A's own frame is the signal
	// that the window we watched was real, and that it is now closed.
	stopA := rewriteOnTicker(t, workspaceWorktreePath(t, h, a), sentinelFile, "edit A\n")
	defer stopA()

	for {
		var frame map[string]any
		require.NoError(t, json.Unmarshal(readTextFrame(t, conn), &frame))
		if statusHasFile(frame, sentinelFile) {
			return // A's sentinel landed and B never leaked: scoping holds
		}
		require.False(t, statusHasFile(frame, "README.md"),
			"A must not receive B's events (leaked frame: %v)", frame)
	}
}

// rewriteOnTicker re-writes name under dir every 100ms with distinct content,
// guaranteeing a filesystem event lands after the lazily started watcher attaches
// its inotify/fsevents hook — without a fixed sleep. The returned stop halts it.
func rewriteOnTicker(
	t *testing.T,
	dir string,
	name string,
	content string,
) func() {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				i++
				_ = writeFile(dir, name, content+strconv.Itoa(i))
			}
		}
	}()
	return func() {
		ticker.Stop()
		close(done)
	}
}
