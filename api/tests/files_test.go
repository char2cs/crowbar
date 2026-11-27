//go:build integration

package tests

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/tests/kit"
)

type documentRef struct {
	Authority   string `json:"authority"`
	WorkspaceID string `json:"workspaceId"`
	Path        string `json:"path"`
}

func TestFiles_TreeSaveReadBack(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var tree []map[string]any
	h.get(base+"/files/tree", &tree)
	require.NotEmpty(t, tree, "tree must list the committed README")

	const content = "package main\n\nfunc main() {}\n"
	var saved documentRef
	h.put(documentsContent, map[string]string{
		"workspaceId": imported.workspaceID, "path": "main.go", "content": content,
	}, &saved)
	assert.Equal(t, documentRef{Authority: "workspace", WorkspaceID: imported.workspaceID, Path: "main.go"}, saved)

	var read struct {
		Content string `json:"content"`
	}
	h.get(docURL(imported.workspaceID, "main.go"), &read)
	assert.Equal(t, content, read.Content)
}

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
	h.get(docURL(imported.workspaceID, "README.md"), &read)
	assert.Equal(t, "hello\n", read.Content)

	h.postError(base+"/files", map[string]string{"path": "a.txt", "type": "file"}, http.StatusConflict)
	h.mutationError(http.MethodPut, documentsContent, map[string]string{
		"workspaceId": imported.workspaceID, "path": "README.md", "content": "x",
	}, http.StatusConflict)
}

func TestRegression_HostDocumentRefusedOverTCP(t *testing.T) {
	h := newHarness(t)
	abs := filepath.Join(t.TempDir(), "secret.md")
	require.NoError(t, os.WriteFile(abs, []byte("secret\n"), 0o600))

	read := refusal(t, h, http.MethodGet, documentsContent+"?path="+url.QueryEscape(abs), nil)
	assert.Equal(t, "host_document_requires_local_transport", read)
	write := refusal(t, h, http.MethodPut, documentsContent, map[string]string{"path": abs, "content": "x"})
	assert.Equal(t, "host_document_requires_local_transport", write)

	got, err := os.ReadFile(abs)
	require.NoError(t, err)
	assert.Equal(t, "secret\n", string(got))
}

func TestRegression_HostDocumentWatchRefusedOverTCP(t *testing.T) {
	h := newHarness(t)
	abs := filepath.Join(t.TempDir(), "a.md")
	scope := "doc:" + abs

	c := dialStream(t, h)
	c.Sub(t, scope, 0)
	closed := c.Next(t, func(f stream.Frame) bool { return f.T == stream.FrameClosed && f.Scope == scope })
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	require.NotNil(t, closed.Problem)
	assert.Equal(t, "host_document_requires_local_transport", string(closed.Problem.Code))
}

const documentChangeBound = 15 * time.Second

func docChange(frame map[string]any) map[string]any {
	if frame["t"] != stream.FrameSignal || frame["kind"] != "file_change" {
		return nil
	}
	data, _ := frame["data"].(map[string]any)
	return data
}

func changeOf(path string) func(map[string]any) bool {
	return func(frame map[string]any) bool {
		ev := docChange(frame)
		return ev != nil && ev["path"] == path
	}
}

func TestRegression_HostDocumentWatchArmedBeforeRead(t *testing.T) {
	h := newHarness(t)
	local := kit.ServeLocal(t, h.server.Config.Handler)
	a := filepath.Join(t.TempDir(), "a.md")
	require.NoError(t, os.WriteFile(a, []byte("v1\n"), 0o600))
	canonA, _ := local.ReadDocument(t, a)

	w := local.HoldDocument(t, canonA)
	require.NoError(t, os.WriteFile(a, []byte("v2\n"), 0o600))

	_, text := local.ReadDocument(t, canonA)
	assert.Equal(t, "v2\n", text)
	w.ReadUntil(t, documentChangeBound, changeOf(canonA))
}

func TestStream_DocScopeSignalsAnExternalChange(t *testing.T) {
	h := newHarness(t)
	local := kit.ServeLocal(t, h.server.Config.Handler)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(a, []byte("a\n"), 0o600))
	require.NoError(t, os.WriteFile(b, []byte("b\n"), 0o600))
	canonA, _ := local.ReadDocument(t, a)
	canonB, _ := local.ReadDocument(t, b)
	wa, wb := local.HoldDocument(t, canonA), local.HoldDocument(t, canonB)

	require.NoError(t, os.WriteFile(a, []byte("external\n"), 0o600))
	frame := wa.ReadUntil(t, documentChangeBound, changeOf(canonA))
	assert.Equal(t, "doc:"+canonA, frame["scope"])
	msg := docChange(frame)
	assert.Equal(t, "host", msg["authority"])
	assert.Equal(t, canonA, msg["path"])
	assert.Contains(t, []any{"modified", "created"}, msg["type"])

	wa.Close(t)
	require.NoError(t, os.WriteFile(a, []byte("after close\n"), 0o600))
	require.NoError(t, os.WriteFile(b, []byte("barrier\n"), 0o600))
	wb.ReadUntil(t, documentChangeBound, func(frame map[string]any) bool {
		ev := docChange(frame)
		require.False(t, ev != nil && ev["path"] == canonA, "a closed document's change reached a client")
		return ev != nil && ev["path"] == canonB
	})
}

func TestRegression_DanglingSymlinkWriteRefused(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	planted := filepath.Join(t.TempDir(), "planted.md")
	require.NoError(t, os.Symlink(planted, filepath.Join(workspaceWorktree(t, h, imported), "notes.md")))

	h.mutationError(http.MethodPut, documentsContent, map[string]string{
		"workspaceId": imported.workspaceID, "path": "notes.md", "content": "x",
	}, http.StatusBadRequest)
	_, err := os.Lstat(planted)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRegression_MissingWorktreeStillOwnsItsPaths(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	worktree := workspaceWorktree(t, h, imported)
	require.NoError(t, os.RemoveAll(worktree))
	abs := filepath.Join(worktree, "README.md")

	h.mutationError(http.MethodPut, documentsContent, map[string]string{"path": abs, "content": "x"}, http.StatusConflict)
	_, err := os.Stat(abs)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestRegression_UnicodeSpellingsAreOneDocument(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	worktree := workspaceWorktree(t, h, imported)
	nfc, nfd := "caf\u00e9.md", "cafe\u0301.md"
	require.NoError(t, os.WriteFile(filepath.Join(worktree, nfd), []byte("x\n"), 0o600))
	if _, err := os.Stat(filepath.Join(worktree, nfc)); err != nil {
		t.Skip("normalization-sensitive volume")
	}

	for _, typed := range []string{nfd, nfc} {
		var doc documentRef
		h.get(documentsContent+"?path="+url.QueryEscape(filepath.Join(worktree, typed)), &doc)
		assert.Equal(t, documentRef{Authority: "workspace", WorkspaceID: imported.workspaceID, Path: nfd}, doc, typed)
	}
}

func refusal(t *testing.T, h *harness, method, path string, body any) string {
	t.Helper()
	resp := h.raw(method, path, body, http.StatusForbidden)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	var env struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	return env.Code
}

func workspaceWorktree(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	for _, ws := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if ws.ID == imported.workspaceID {
			require.NotEmpty(t, ws.LocalPath, "workspace list must expose localPath")
			return ws.LocalPath
		}
	}
	require.FailNow(t, "imported workspace missing from list")
	return ""
}

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

func TestRegression_BinaryUploadByteFaithful(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	worktree := workspaceWorktree(t, h, imported)

	binary := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0xFF, 0xFE, 0x01, 0x80}
	encoded := base64.StdEncoding.EncodeToString(binary)

	h.put(documentsContent, map[string]string{
		"workspaceId": imported.workspaceID, "path": "upload.bin", "content": encoded, "encoding": "base64",
	}, nil)

	got, err := os.ReadFile(filepath.Join(worktree, "upload.bin"))
	require.NoError(t, err)
	assert.Equal(t, binary, got, "uploaded binary must land byte-identical on disk")

	var read struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	h.get(docURL(imported.workspaceID, "upload.bin"), &read)
	assert.Equal(t, "base64", read.Encoding)
	assert.Equal(t, encoded, read.Content)

	h.put(documentsContent, map[string]string{
		"workspaceId": imported.workspaceID, "path": "note.txt", "content": "héllo\n",
	}, nil)
	text, err := os.ReadFile(filepath.Join(worktree, "note.txt"))
	require.NoError(t, err)
	assert.Equal(t, "héllo\n", string(text))
}

func TestFiles_CopyDirectoryRecursiveAndConflicts(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/files", map[string]string{"path": "src/nested", "type": "dir"}, http.StatusCreated, &created)
	h.put(documentsContent, map[string]string{
		"workspaceId": imported.workspaceID, "path": "src/nested/a.txt", "content": "deep",
	}, nil)

	var copied struct {
		ID string `json:"id"`
	}
	h.post(base+"/files/copy", map[string]string{"sourcePath": "src", "destPath": "src copy"},
		http.StatusCreated, &copied)
	assert.Equal(t, "src copy", copied.ID)

	var read struct {
		Content string `json:"content"`
	}
	h.get(docURL(imported.workspaceID, "src copy/nested/a.txt"), &read)
	assert.Equal(t, "deep", read.Content)

	h.postError(base+"/files/copy", map[string]string{"sourcePath": "src", "destPath": "src copy"},
		http.StatusConflict)
	h.postError(base+"/files/copy", map[string]string{"sourcePath": "ghost", "destPath": "ghost copy"},
		http.StatusNotFound)
}
