//go:build integration

package tests

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A project's home folders (RepoID == "") are the ONLY rows a project's
// GET/PATCH/DELETE .../home/chats/folders may see or touch. domain.Folder
// carries no project id, so every project's home folders share the one
// RepoID == "" bucket ListInRepo("") filters on.
func TestRegression_HomeFoldersAreProjectScoped(t *testing.T) {
	h := newHarness(t)
	projectA := createProjectForHome(t, h)
	projectB := createProjectForHome(t, h)
	require.NotEqual(t, projectA, projectB)

	baseA := "/v0/projects/" + projectA + "/home"
	baseB := "/v0/projects/" + projectB + "/home"

	bOnly := createChatFolder(t, h, baseB, "B-only", "")
	h.Quiesce()

	inB := listChatFolders(t, h, baseB)
	_, ok := chatFolderByID(inB, bOnly.ID)
	require.True(t, ok, "B's own list must carry B-only")

	inA := listChatFolders(t, h, baseA)
	_, leaked := chatFolderByID(inA, bOnly.ID)
	assert.False(t, leaked,
		"project A's home folder list must not carry project B's folder; got %+v", inA)

	// The write side is unscoped by the same id: a PATCH through A's mount
	// renames B's folder, and a DELETE through A's mount removes it.
	status := crossProjectRequest(t, h, http.MethodPatch, baseA+"/chats/folders/"+bOnly.ID,
		`{"name":"renamed-from-A"}`)
	t.Logf("PATCH via A on B's folder -> %d", status)
	assert.Equal(t, http.StatusNotFound, status,
		"project A must not be able to patch project B's home folder")

	status = crossProjectRequest(t, h, http.MethodDelete, baseA+"/chats/folders/"+bOnly.ID, "")
	t.Logf("DELETE via A on B's folder -> %d", status)
	assert.Equal(t, http.StatusNotFound, status,
		"project A must not be able to delete project B's home folder")
	h.Quiesce()

	inB = listChatFolders(t, h, baseB)
	row, ok := chatFolderByID(inB, bOnly.ID)
	require.True(t, ok, "B's folder must survive a cross-project delete")
	assert.Equal(t, "B-only", row.Title, "B's folder must survive a cross-project rename")
}

func crossProjectRequest(
	t *testing.T,
	h *harness,
	method string,
	path string,
	body string,
) int {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, h.url+path, reader)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	_ = resp.Body.Close()
	return resp.StatusCode
}
