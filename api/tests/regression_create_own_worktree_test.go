//go:build integration

package tests

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createChatWithOwnWorktree(
	t *testing.T,
	h *harness,
	base string,
	provider string,
	parentID string,
) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats",
		map[string]any{"provider": provider, "parentId": parentID, "ownWorktree": true},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "create must respond with the new chat's id")
	h.QuiesceReactors()
	return created.ID
}

func TestRegression_CreateChatWithOwnWorktreeIsAtomicOverHTTP(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	ancestorID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, "")
	startChat(t, h, base, ancestorID)
	ancestorCwd := readSpawnCwd(t, h, imported, ancestorID)
	require.NotEmpty(t, ancestorCwd, "the ancestor's own spawn must report a real cwd")

	folder := createChatFolder(t, h, base, "own-worktree-holder", ancestorID)

	newChatID := createChatWithOwnWorktree(t, h, base, "promotestub", folder.ID)

	detail := getAgentChat(t, h, base, newChatID)
	assert.Equal(t, folder.ID, detail.ParentID, "the create keeps the chat exactly where it was asked to be born")
	require.NotEmpty(t, detail.WorkspaceID, "ownWorktree must fill the workspace slot in the SAME call")
	assert.NotEqual(t, imported.workspaceID, detail.WorkspaceID,
		"the workspace must be a FRESH one forked from the resolved fork parent, not the ancestor's own")
	startChat(t, h, base, newChatID)

	newCwd := readSpawnCwd(t, h, imported, newChatID)
	require.NotEmpty(t, newCwd, "the new chat's CLI must report a real cwd — a real worktree on disk")
	assert.NotEqual(t, ancestorCwd, newCwd,
		"the new chat's CLI must run in its OWN new worktree, not the ancestor's")
}

func TestRegression_CreateChatOwnWorktreeIsIgnoredWhenWorkspaceIdIsSupplied(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	var created struct {
		ID string `json:"id"`
	}
	h.post(base+"/chats",
		map[string]any{"provider": "promotestub", "workspaceId": imported.workspaceID, "ownWorktree": true},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()

	detail := getAgentChat(t, h, base, created.ID)
	assert.Equal(t, imported.workspaceID, detail.WorkspaceID,
		"a request naming an existing workspace must attach to it, not fork a new one")
}

func TestRegression_CreateChatOwnWorktreeWithNoProviderIs400(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)
	ancestorID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, "")
	folder := createChatFolder(t, h, base, "own-worktree-holder", ancestorID)

	resp := h.raw(http.MethodPost, base+"/chats",
		map[string]any{"parentId": folder.ID, "ownWorktree": true}, http.StatusBadRequest)
	defer func() { require.NoError(t, resp.Body.Close()) }()
	var env struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	assert.Equal(t, "invalid_argument", env.Code)
}

func TestHome_EverythingCrowbarWritesIsOwnerOnly(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)
	ancestorID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, "")
	startChat(t, h, base, ancestorID)
	folder := createChatFolder(t, h, base, "own-worktree-holder", ancestorID)
	forkID := createChatWithOwnWorktree(t, h, base, "promotestub", folder.ID)
	startChat(t, h, base, forkID)
	src := filepath.Join(t.TempDir(), "notes.txt")
	require.NoError(t, os.WriteFile(src, []byte("an attachment"), 0o600))
	h.post(base+"/chats/"+forkID+"/attachments", map[string]string{"path": src, "id": "att1"}, http.StatusCreated, nil)
	h.Quiesce()

	way := map[string]bool{}
	require.NoError(t, filepath.WalkDir(h.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || d.Name() != "worktree" {
			return err
		}
		for dir := filepath.Dir(path); !way[dir] && dir != filepath.Dir(h.home); dir = filepath.Dir(dir) {
			way[dir] = true
		}
		return fs.SkipDir
	}))
	require.NotEmpty(t, way, "the fork has a checkout under the home")
	var loose []string
	require.NoError(t, filepath.WalkDir(h.home, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "worktree" {
			return fs.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		want := fs.FileMode(0o600)
		switch {
		case d.IsDir() && way[path]:
			want = 0o711
		case d.IsDir():
			want = 0o700
		case !info.Mode().IsRegular():
			return nil
		}
		if info.Mode().Perm() != want {
			rel, _ := filepath.Rel(h.home, path)
			loose = append(loose, rel+" "+info.Mode().Perm().String())
		}
		return nil
	}))
	assert.Empty(t, loose, "files Crowbar wrote that are not owner-only:\n%s", strings.Join(loose, "\n"))
}
