//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_RefusedCreateCarriesItsCode: a create the daemon refuses says why in a code
// the client can branch on, not only in prose.
func TestRegression_RefusedCreateCarriesItsCode(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	f := newImportFixture(t, h)

	const branch = "feature/a"
	f.pushBranchToOrigin(t, branch, "main", "a\n")
	conn := h.dial(f.repoBase() + "/chats/ws")
	require.NoError(t, h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close())
	readUntil(t, conn, func(m map[string]any) bool { return m["kind"] == "workspace_set" })
	h.Quiesce()

	body, err := json.Marshal(map[string]any{
		"provider": "promotestub", "parentId": f.mainWSID, "ownWorktree": true, "branch": branch,
	})
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, h.url+f.repoBase()+"/chats", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	require.NoError(t, err)
	defer func() { require.NoError(t, resp.Body.Close()) }()

	var env struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	assert.Equal(t, "conflict", env.Code)
	assert.Contains(t, env.Error, branch)
}
