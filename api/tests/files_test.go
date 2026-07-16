//go:build integration

package tests

import (
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
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
	base := wsBase(imported)

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
	base := wsBase(imported)

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
	base := wsBase(imported)

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

// workspaceWorktree resolves the on-disk worktree of the imported child
// workspace from the workspace list (WorkspaceDTO.localPath), so a test can
// seed content the HTTP surface cannot express — e.g. real binary bytes.
func workspaceWorktree(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	type wsWithPath struct {
		ID        string `json:"id"`
		LocalPath string `json:"localPath"`
	}
	var workspaces []wsWithPath
	h.get("/v0/projects/"+imported.projectID+"/repos/"+imported.repoID+"/workspaces", &workspaces)
	for _, ws := range workspaces {
		if ws.ID == imported.workspaceID {
			require.NotEmpty(t, ws.LocalPath, "workspace list must expose localPath")
			return ws.LocalPath
		}
	}
	require.FailNow(t, "imported workspace missing from list")
	return ""
}

// TestFiles_CopyBinaryByteFaithful proves the copy verb is a server-side byte
// copy: a binary file (invalid UTF-8, contains NULs) seeded on disk comes out
// byte-identical. This is exactly the case the base64 content read corrupts
// when a client round-trips it through GET+PUT content — the reason the verb
// exists (Task 28).
func TestFiles_CopyBinaryByteFaithful(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)
	worktree := workspaceWorktree(t, h, imported)

	binary := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0xFF, 0xFE, 0x01}
	require.NoError(t, os.WriteFile(filepath.Join(worktree, "img.png"), binary, 0o600))

	var copied struct {
		ID string `json:"id"`
	}
	h.post(base+"/files/copy",
		map[string]string{"sourcePath": "img.png", "destPath": "img copy.png"},
		http.StatusCreated, &copied)
	assert.Equal(t, "img copy.png", copied.ID)

	got, err := os.ReadFile(filepath.Join(worktree, "img copy.png"))
	require.NoError(t, err)
	assert.Equal(t, binary, got, "copy must be byte-identical to the source")
}

// TestRegression_BinaryUploadByteFaithful proves the content-write verb honors a
// base64 encoding, so file UPLOAD (read a File as bytes → base64 → PUT content)
// lands byte-identical on disk. Before the encoding field the write path decoded
// nothing and wrote the raw string bytes, corrupting any binary (invalid UTF-8,
// embedded NULs) the moment a client round-tripped it (Task 28/34 escalation).
func TestRegression_BinaryUploadByteFaithful(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)
	worktree := workspaceWorktree(t, h, imported)

	binary := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0xFF, 0xFE, 0x01, 0x80}
	encoded := base64.StdEncoding.EncodeToString(binary)

	var saved struct {
		ID string `json:"id"`
	}
	h.put(base+"/files/content",
		map[string]string{"path": "upload.bin", "content": encoded, "encoding": "base64"},
		&saved)

	got, err := os.ReadFile(filepath.Join(worktree, "upload.bin"))
	require.NoError(t, err)
	assert.Equal(t, binary, got, "uploaded binary must land byte-identical on disk")

	// It also round-trips back out through the content read as base64.
	var read struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	h.get(base+"/files/content?path=upload.bin", &read)
	assert.Equal(t, "base64", read.Encoding)
	assert.Equal(t, encoded, read.Content)

	// A plain-text PUT with no encoding still writes UTF-8 unchanged (the
	// default path is untouched by the new field).
	var textSaved struct {
		ID string `json:"id"`
	}
	h.put(base+"/files/content",
		map[string]string{"path": "note.txt", "content": "héllo\n"},
		&textSaved)
	text, err := os.ReadFile(filepath.Join(worktree, "note.txt"))
	require.NoError(t, err)
	assert.Equal(t, "héllo\n", string(text))
}

// TestFiles_CopyDirectoryRecursiveAndConflicts drives the directory case over
// the HTTP surface: a nested dir copies recursively (content readable at the
// copied path), a second copy onto the same destination is a 409, and a
// missing source is a 404.
func TestFiles_CopyDirectoryRecursiveAndConflicts(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/files", map[string]string{"path": "src/nested", "type": "dir"}, http.StatusCreated, &created)
	var saved struct {
		ID string `json:"id"`
	}
	h.put(base+"/files/content", map[string]string{"path": "src/nested/a.txt", "content": "deep"}, &saved)

	var copied struct {
		ID string `json:"id"`
	}
	h.post(base+"/files/copy", map[string]string{"sourcePath": "src", "destPath": "src copy"},
		http.StatusCreated, &copied)
	assert.Equal(t, "src copy", copied.ID)

	var read struct {
		Content string `json:"content"`
	}
	h.get(base+"/files/content?path=src+copy%2Fnested%2Fa.txt", &read)
	assert.Equal(t, "deep", read.Content)

	h.postError(base+"/files/copy", map[string]string{"sourcePath": "src", "destPath": "src copy"},
		http.StatusConflict)
	h.postError(base+"/files/copy", map[string]string{"sourcePath": "ghost", "destPath": "ghost copy"},
		http.StatusNotFound)
}
