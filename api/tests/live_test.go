//go:build integration

package tests

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// TestStream_GitScopeSignalsAStatusChange proves the watcher chain on the git
// scope: the current status arrives on hold, then a real on-disk edit
// (re-written on a ticker so the test never races the lazily started watcher)
// is signalled carrying the dirty file.
func TestStream_GitScopeSignalsAStatusChange(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	scope := "git:" + imported.workspaceID

	c := dialStream(t, h)
	c.Sub(t, scope, 0)
	snapshot := c.Signal(t, scope, "git_status", func(m map[string]any) bool { return m["branch"] != nil })
	assert.Equal(t, "main", snapshot["branch"])

	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, imported), "README.md", "snapshot then live\n")
	defer stop()

	c.Signal(t, scope, "git_status", func(m map[string]any) bool { return statusHasFile(m, "README.md") })
}

// TestStream_FilesScopeSignalsAFileChange proves the watcher chain on the files
// scope: a real edit is signalled as a FileChangeEvent of that workspace.
func TestStream_FilesScopeSignalsAFileChange(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	scope := "files:" + imported.workspaceID

	c := dialStream(t, h)
	c.Hold(t, scope)
	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, imported), "watched.txt", "edit\n")
	defer stop()

	got := c.Signal(t, scope, "file_change", func(m map[string]any) bool { return m["path"] != nil })
	assert.Equal(t, imported.workspaceID, got["wsId"])
}

// TestStream_FilesScopeSurvivesASubUnsubSub: a mount's back-to-back sub,
// unsub and sub of the same scope leave it held and its watcher running.
func TestStream_FilesScopeSurvivesASubUnsubSub(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	scope := "files:" + imported.workspaceID

	c := dialStream(t, h)
	c.Sub(t, scope, 0)
	c.Unsub(t, scope)
	c.Sub(t, scope, 0)
	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, imported), "churned.txt", "edit\n")
	defer stop()

	got := c.Signal(t, scope, "file_change", func(m map[string]any) bool { return m["path"] == "churned.txt" })
	assert.Equal(t, imported.workspaceID, got["wsId"])
}

// TestStream_GitScopeIsolatesWorkspaces: a client holding A's git scope never
// hears B's status, even while B is edited. A sentinel edit in A bounds the
// watch: reaching it having seen only A's scope proves isolation, with no clock.
func TestStream_GitScopeIsolatesWorkspaces(t *testing.T) {
	h := newHarness(t)
	a := importProject(t, h)
	b := importProject(t, h)
	scopeA, scopeB := "git:"+a.workspaceID, "git:"+b.workspaceID

	ca := dialStream(t, h)
	ca.Hold(t, scopeA)
	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, b), "README.md", "edit B\n")
	defer stop()
	// B's own holder proves B is live and signalling for the rest of the test.
	cb := dialStream(t, h)
	cb.Sub(t, scopeB, 0)
	cb.Signal(t, scopeB, "git_status", func(m map[string]any) bool { return statusHasFile(m, "README.md") })

	stopA := rewriteOnTicker(t, workspaceWorktreePath(t, h, a), sentinelFile, "edit A\n")
	defer stopA()
	ca.Next(t, func(f stream.Frame) bool {
		require.NotEqual(t, scopeB, f.Scope, "A must not receive B's signals")
		if f.T != stream.FrameSignal {
			return false
		}
		var m map[string]any
		require.NoError(t, json.Unmarshal(f.Data, &m))
		return statusHasFile(m, sentinelFile)
	})
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
