//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type gitStatusDTO struct {
	Branch string `json:"branch"`
	Files  []struct {
		Path   string `json:"path"`
		Status string `json:"status"`
	} `json:"files"`
}

// TestGit_StatusDiffStageCommit walks the git working-tree lifecycle over a real
// repo: an edit shows up in status and diff, staging then committing it clears the
// working tree, and the post-commit status reflects a clean branch.
func TestGit_StatusDiffStageCommit(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var saved struct {
		ID string `json:"id"`
	}
	h.put(base+"/files/content", map[string]string{"path": "README.md", "content": "hello\nworld\n"}, &saved)

	var status gitStatusDTO
	h.get(base+"/git/status", &status)
	assert.Equal(t, "feature/write", status.Branch)
	require.NotEmpty(t, status.Files, "the edit must appear in git status")

	var diffs []map[string]any
	h.get(base+"/git/diff", &diffs)
	require.NotEmpty(t, diffs, "the edit must appear in the working-tree diff")

	var staged struct {
		ID string `json:"id"`
	}
	h.post(base+"/git/stage", map[string]any{"paths": []string{"README.md"}}, http.StatusOK, &staged)

	var committed struct {
		ID string `json:"id"`
	}
	h.post(base+"/git/commit", map[string]string{"subject": "update readme"}, http.StatusOK, &committed)
	assert.Equal(t, imported.workspaceID, committed.ID)

	var after gitStatusDTO
	h.get(base+"/git/status", &after)
	assert.Empty(t, after.Files, "working tree must be clean after commit")
}

// TestGit_LockedWorkspaceRejectsWrite proves the locked-workspace guard: the
// adopted "main" workspace is locked (a default protected branch), so reads
// still succeed (200) while a git mutation is rejected with 409 (04 §5, 05 §3).
func TestGit_LockedWorkspaceRejectsWrite(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := wsBase(imported)

	// The dedicated GET .../workspaces/:wsId detail route is gone (spec §8 step
	// 6); worktreeOf reads the same answer off the chat list.
	detail := worktreeOf(t, h, imported, imported.workspaceID)
	require.Equal(t, "locked", detail.Status, "adopted main worktree must be locked")

	var status gitStatusDTO
	h.get(base+"/git/status", &status)
	assert.Equal(t, "main", status.Branch)

	h.postError(base+"/git/commit", map[string]string{"subject": "blocked"}, http.StatusConflict)
	h.postError(base+"/git/stage", map[string]any{"paths": []string{"README.md"}}, http.StatusConflict)
}

// TestGit_StatusDualServeWS proves the /git/status route dual-serves: a WebSocket
// upgrade delivers the GitStatus snapshot on connect.
func TestGit_StatusDualServeWS(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	conn := h.dial(wsBase(imported) + "/git/status")
	got := readUntil(t, conn, func(m map[string]any) bool {
		_, ok := m["branch"]
		return ok
	})
	assert.Equal(t, "main", got["branch"])
}
