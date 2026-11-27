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

func TestStream_GitScopeIsolatesWorkspaces(t *testing.T) {
	h := newHarness(t)
	a := importProject(t, h)
	b := importProject(t, h)
	scopeA, scopeB := "git:"+a.workspaceID, "git:"+b.workspaceID

	ca := dialStream(t, h)
	ca.Hold(t, scopeA)
	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, b), "README.md", "edit B\n")
	defer stop()
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
