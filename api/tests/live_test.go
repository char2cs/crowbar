//go:build integration

package tests

import (
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

	stop := rewriteOnTicker(t, imported.repoPath, "README.md", "snapshot then live\n")
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

	stop := rewriteOnTicker(t, imported.repoPath, "watched.txt", "edit\n")
	defer stop()

	got := readUntil(t, conn, func(m map[string]any) bool {
		_, ok := m["path"]
		return ok
	})
	assert.Equal(t, imported.workspaceID, got["wsId"])
}

// TestLive_WorkspaceSnapshotOnConnect proves snapshot-on-subscribe for the
// repo-scoped Workspaces topic: the current workspace row is delivered
// immediately on connect, before any live event, so the sidebar renders live on
// first connect.
func TestLive_WorkspaceSnapshotOnConnect(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	conn := h.dial("/v0/projects/" + imported.projectID + "/repos/" + imported.repoID + "/workspaces")
	got := readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID
	})
	assert.Equal(t, imported.repoID, got["repoId"])
}

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

	// Drain A's snapshot so the subsequent read window is for live events only.
	readUntil(t, conn, func(m map[string]any) bool {
		_, ok := m["branch"]
		return ok
	})

	stop := rewriteOnTicker(t, b.repoPath, "README.md", "edit B\n")
	defer stop()

	// Subscribing to B's watcher via a second connection makes B's events real;
	// the A-scoped connection must still never see a dirty (non-snapshot) frame.
	bConn := h.dial(wsBase(b) + "/git/status")
	readUntil(t, bConn, func(m map[string]any) bool {
		files, ok := m["files"].([]any)
		return ok && len(files) > 0
	})

	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	for {
		mt, _, err := conn.ReadMessage()
		if err != nil {
			return // deadline elapsed with no leak from B: scoping holds
		}
		require.NotEqual(t, websocket.TextMessage, mt, "A must not receive B's events")
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
