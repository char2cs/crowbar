//go:build integration

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every sidebar folder written before #179 is a `folders` row (the SAME GORM
// table domain.Folder still reads) with its position in the retired
// parent_id/order columns and NO Node row. The read side tolerates that
// (listFolders degrades to a zero Node), so the sidebar draws the folder at
// the root — but Move and Delete resolve the Node first and fail outright.
func legacyNodelessFolder(t *testing.T, h *harness, base, name string) string {
	t.Helper()
	f := createChatFolder(t, h, base, name, "")
	require.NoError(t, h.app.Repositories.Node.Forget(context.Background(), f.ID))
	h.Quiesce()
	return f.ID
}

// statusOf issues a mutation and returns its status and body, asserting nothing.
func statusOf(t *testing.T, h *harness, method, path string, body any) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, h.url+path, reader)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	text, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(text)
}

func assertLegacyFolderVerbs(t *testing.T, h *harness, base, legacy string) {
	t.Helper()
	_, listed := chatFolderByID(listChatFolders(t, h, base), legacy)
	require.True(t, listed, "the sidebar draws the legacy folder")

	status, body := statusOf(t, h, http.MethodPatch, base+"/chats/folders/"+legacy, map[string]any{"order": 0})
	assert.Equalf(t, http.StatusOK, status, "reorder of a legacy folder: %s", body)

	status, body = statusOf(t, h, http.MethodPatch, base+"/chats/folders/"+legacy, map[string]any{"name": "renamed"})
	assert.Equalf(t, http.StatusOK, status, "rename (which also runs Move) of a legacy folder: %s", body)

	status, body = statusOf(t, h, http.MethodDelete, base+"/chats/folders/"+legacy, nil)
	assert.Equalf(t, http.StatusOK, status, "delete of a legacy folder: %s", body)
}

func TestRegression_LegacyNodelessHomeFolderCanBeMovedAndDeleted(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	p := createProjectForHome(t, h)
	base := "/v0/projects/" + p + "/home"
	assertLegacyFolderVerbs(t, h, base, legacyNodelessFolder(t, h, base, "legacy"))
}

func TestRegression_LegacyNodelessRepoFolderCanBeMovedAndDeleted(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)
	assertLegacyFolderVerbs(t, h, base, legacyNodelessFolder(t, h, base, "legacy"))
}
