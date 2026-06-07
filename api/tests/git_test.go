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
	imported := importProject(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	require.NoError(t, writeFile(imported.repoPath, "README.md", "hello\nworld\n"))

	var status gitStatusDTO
	h.get(base+"/git/status", &status)
	assert.Equal(t, "main", status.Branch)
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

// TestGit_StatusDualServeWS proves the /git/status route dual-serves: a WebSocket
// upgrade delivers the GitStatus snapshot on connect.
func TestGit_StatusDualServeWS(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	conn := h.dial("/v0/workspaces/" + imported.workspaceID + "/git/status")
	got := readUntil(t, conn, func(m map[string]any) bool {
		_, ok := m["branch"]
		return ok
	})
	assert.Equal(t, "main", got["branch"])
}
