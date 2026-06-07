//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFiles_TreeSaveReadBack proves the file read/write round trip over a real
// worktree: the tree lists the committed README, a save writes new content, and a
// subsequent read returns exactly what was written.
func TestFiles_TreeSaveReadBack(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	var tree []map[string]any
	h.get(base+"/files/tree", &tree)
	require.NotEmpty(t, tree, "tree must list the committed README")

	const content = "package main\n\nfunc main() {}\n"
	var saved struct {
		ID string `json:"id"`
	}
	h.put(base+"/files/content", map[string]string{"path": "main.go", "content": content}, &saved)
	assert.Equal(t, "main.go", saved.ID)

	var read struct {
		Content string `json:"content"`
	}
	h.get(base+"/files/content?path=main.go", &read)
	assert.Equal(t, content, read.Content)
}

// TestFiles_CreateRenameDelete walks the file mutation lifecycle: create a file,
// rename it, then delete it — each step echoing its path through the envelope.
func TestFiles_CreateRenameDelete(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/files", map[string]string{"path": "a.txt", "type": "file"}, http.StatusCreated, &created)
	assert.Equal(t, "a.txt", created.ID)

	var renamed struct {
		ID string `json:"id"`
	}
	h.patch(base+"/files", map[string]string{"path": "a.txt", "newPath": "b.txt"}, &renamed)
	assert.Equal(t, "b.txt", renamed.ID)

	var deleted struct {
		ID string `json:"id"`
	}
	h.del(base+"/files", map[string]string{"path": "b.txt"}, http.StatusOK, &deleted)
	assert.Equal(t, "b.txt", deleted.ID)
}

// TestFiles_LockedWorkspaceRejectsWrite proves the locked-workspace guard on the
// file write path: the adopted "main" workspace is locked (a default protected
// branch), so reads still succeed (200) while a file create is rejected with 409
// (04 §5, 05 §3/§4).
func TestFiles_LockedWorkspaceRejectsWrite(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	var tree []map[string]any
	h.get(base+"/files/tree", &tree)
	require.NotEmpty(t, tree, "reads must still succeed on a locked workspace")

	var read struct {
		Content string `json:"content"`
	}
	h.get(base+"/files/content?path=README.md", &read)
	assert.Equal(t, "hello\n", read.Content)

	h.postError(base+"/files", map[string]string{"path": "a.txt", "type": "file"}, http.StatusConflict)
}
