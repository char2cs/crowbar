package v0_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
)

// seedSearchFile writes a file to the workspace worktree path for search tests.
func seedSearchFile(
	t *testing.T,
	worktreePath string,
	rel string,
	content string,
) {
	t.Helper()
	abs := filepath.Join(worktreePath, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o600))
}

func TestSearch_NoEngine_503(t *testing.T) {
	tc := newApp(t)
	tc.eng.Search = nil
	_, r := newRouter(t, tc)

	body := bytes.NewBufferString(`{"query":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/ws1/search", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestSearch_WorkspaceNotFound_404(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	body := bytes.NewBufferString(`{"query":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/no-such/search", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSearch_EmptyQuery_400(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := seedWorkspace(t, tc, "ws-search-empty")

	body := bytes.NewBufferString(`{"query":""}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/search", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearch_BadRegex_400(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := seedWorkspace(t, tc, "ws-search-badre")

	body := bytes.NewBufferString(`{"query":"[invalid","regex":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/search", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearch_HappyPath_200(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := seedWorkspace(t, tc, "ws-search-ok")

	// Find the worktree path from the seeded workspace.
	var worktreePath string
	{
		ws, err := tc.app.Repositories.Workspace.Get(t.Context(), wsID)
		require.NoError(t, err)
		worktreePath = ws.WorktreePath
	}
	seedSearchFile(t, worktreePath, "src/main.go", "hello world\n")

	body := bytes.NewBufferString(`{"query":"hello","caseSensitive":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/search", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	results := resp["results"].([]any)
	assert.NotEmpty(t, results)
}

// --- Replace ---

func TestReplace_NoEngine_503(t *testing.T) {
	tc := newApp(t)
	tc.eng.Search = nil
	_, r := newRouter(t, tc)

	body := bytes.NewBufferString(`{"query":"hello","replacement":"bye","scope":"all"}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/ws1/search/replace", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestReplace_WorkspaceNotFound_404(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)

	body := bytes.NewBufferString(`{"query":"hello","replacement":"bye","scope":"all"}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/no-such/search/replace", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestReplace_BadPattern_400(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := seedWorkspace(t, tc, "ws-replace-badre")

	body := bytes.NewBufferString(`{"query":"[invalid","replacement":"x","scope":"all","regex":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/search/replace", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestReplace_HappyPath_200(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := seedWorkspace(t, tc, "ws-replace-ok")

	var worktreePath string
	{
		ws, err := tc.app.Repositories.Workspace.Get(t.Context(), wsID)
		require.NoError(t, err)
		worktreePath = ws.WorktreePath
	}
	seedSearchFile(t, worktreePath, "file.go", "hello world\n")

	body := bytes.NewBufferString(`{"query":"hello","replacement":"goodbye","scope":"all","caseSensitive":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/search/replace", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	data, err := os.ReadFile(filepath.Join(worktreePath, "file.go"))
	require.NoError(t, err)
	assert.Equal(t, "goodbye world\n", string(data))
}

func TestReplace_LockedWorkspace_403(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := "ws-replace-locked"

	// Create a locked workspace.
	dir := t.TempDir()
	_, err := tc.app.Repositories.Workspace.Create(
		context.Background(),
		workspace.CreateInput{
			ID:           wsID,
			RepoID:       "r1",
			ProjectID:    "p1",
			WorktreePath: dir,
			Locked:       true,
		},
		time.Now(),
	)
	require.NoError(t, err)
	seedSearchFile(t, dir, "file.go", "hello\n")

	body := bytes.NewBufferString(`{"query":"hello","replacement":"bye","scope":"all","caseSensitive":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/search/replace", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestReplace_PathTraversal_400(t *testing.T) {
	tc := newApp(t)
	_, r := newRouter(t, tc)
	wsID := seedWorkspace(t, tc, "ws-replace-traversal")

	body := bytes.NewBufferString(`{"query":"hello","replacement":"bye","scope":"file:../../etc/passwd","caseSensitive":true}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/workspaces/"+wsID+"/search/replace", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
