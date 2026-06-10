//go:build integration

package tests

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// This file pins backend contracts that broke in the field during the UX QA
// loop (2026-06-10). Each test names the bug it guards against. If one of
// these fails, the frontend is broken in the corresponding way even when the
// rest of the suite passes.

// BUG-001: the file tree must be served at GET /files/tree. The backend once
// registered it at GET /files, which 404'd every file-explorer load while all
// other git/files routes kept working.
func TestRegression_FilesTreeServedAtTreePath(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	var tree []struct {
		Name string `json:"name"`
		Path string `json:"path"`
		Type string `json:"type"`
	}
	h.get(base+"/files/tree", &tree)
	require.NotEmpty(t, tree, "tree at /files/tree must list the repo root")

	// The bare files path is the mutation route (POST/PATCH/DELETE); a GET on
	// it must not silently serve the tree under a second path.
	resp := h.raw(http.MethodGet, base+"/files", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

// BUG-002: every v0 REST endpoint must wrap its payload in the
// {success,error,data} envelope. The files/git/chats/terminal/agent-run groups
// once returned bare payloads, which the frontend's envelope-unwrapping fetch
// rejected wholesale — entire panels rendered empty with 200s on the wire.
// h.get fails the test unless the response carries a success envelope.
func TestRegression_AllReadEndpointsUseEnvelope(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	paths := []string{
		"/v0/projects",
		"/v0/repos",
		"/v0/workspaces",
		base,
		base + "/files/tree",
		base + "/files/content?path=README.md",
		base + "/git/status",
		base + "/git/log?limit=10&skip=0",
		base + "/git/branches",
		base + "/git/stashes",
		base + "/chats",
		"/v0/settings/terminal/profiles",
		"/v0/runs/running",
	}
	for _, path := range paths {
		h.get(path, nil)
	}
}

// BUG-010: git stage, unstage, and discard take {paths: []string} — including
// "." for everything — matching the frontend. The handlers once bound a
// singular {path}, so every stage click 400'd.
func TestRegression_StageUnstageDiscardAcceptPathsArray(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	dirtyWorkspaceFile(t, h, imported.workspaceID, "README.md")

	h.post(base+"/git/stage", map[string]any{"paths": []string{"README.md"}}, http.StatusOK, nil)
	require.True(t, fileStaged(gitStatusFiles(t, h, base), "README.md"),
		"stage {paths:[file]} must stage the file")

	h.post(base+"/git/unstage", map[string]any{"paths": []string{"."}}, http.StatusOK, nil)
	require.False(t, fileStaged(gitStatusFiles(t, h, base), "README.md"),
		`unstage {paths:["."]} must unstage everything`)

	h.post(base+"/git/discard", map[string]any{"paths": []string{"."}}, http.StatusOK, nil)
	require.Empty(t, gitStatusFiles(t, h, base),
		`discard {paths:["."]} must leave a clean working tree`)
}

// BUG-010: commit takes {subject, body}, composed as subject + blank line +
// body. The handler once bound {message}, so every commit from the UI 400'd.
func TestRegression_CommitAcceptsSubjectAndBody(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := "/v0/workspaces/" + imported.workspaceID

	dirtyWorkspaceFile(t, h, imported.workspaceID, "README.md")
	h.post(base+"/git/stage", map[string]any{"paths": []string{"README.md"}}, http.StatusOK, nil)
	h.post(base+"/git/commit", map[string]string{
		"subject": "regression: subject line",
		"body":    "regression body paragraph",
	}, http.StatusOK, nil)

	var log []struct {
		Message     string `json:"message"`
		Description string `json:"description"`
	}
	h.get(base+"/git/log?limit=1&skip=0", &log)
	require.Len(t, log, 1)
	require.Equal(t, "regression: subject line", log[0].Message)
	require.Contains(t, log[0].Description, "regression body paragraph")
}

// BUG-A (git store crash): a clean working tree must serialise git status with
// "files": [] — never "files": null. The handler once wrote the domain object
// straight through, whose nil slice marshalled as null and crashed the
// frontend's store write (History stuck on "Loading…" for clean workspaces).
// Raw-body assertion on purpose: a typed decode can't tell null from [].
func TestRegression_GitStatusFilesNeverNull(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	resp := h.raw(http.MethodGet, "/v0/workspaces/"+imported.workspaceID+"/git/status", nil, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.NotContains(t, string(body), `"files":null`,
		"clean-tree git status must serialise files as [], not null")
	require.Contains(t, string(body), `"files":[]`,
		"clean-tree git status must carry an explicit empty files array")
}

// Broadcast storm: with no working-tree activity, the git topic must stay
// quiet after the subscribe snapshot. Watchers of workspaces sharing a .git
// once re-broadcast an unchanged status on every shared ref event (~6Hz
// identical frames), starving the frontend's refresh debounce. The snapshot
// frame must also serialise files as [] (never null), like the REST DTO.
func TestRegression_GitTopicQuietWhenIdle(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	conn := h.dial("/v0/ws/git?wsId=" + imported.workspaceID)

	// Snapshot-on-subscribe frame arrives first and must carry files: [].
	deadline := time.Now().Add(5 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	mt, raw, err := conn.ReadMessage()
	require.NoError(t, err, "subscribe snapshot frame must arrive")
	require.Equal(t, websocket.TextMessage, mt)
	require.NotContains(t, string(raw), `"files":null`,
		"snapshot frame must serialise files as [], not null")

	// With zero activity the topic must now stay quiet. Tolerate at most one
	// straggler from watcher startup; an identical-frame stream is the bug.
	extra := 0
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break // read deadline: the quiet we expect
		}
		extra++
		require.LessOrEqual(t, extra, 1,
			"idle workspace must not stream repeated git status frames")
	}
}

// dirtyWorkspaceFile appends to a tracked file in the workspace's worktree on
// disk, bypassing the API, so staging tests start from a known-dirty tree.
func dirtyWorkspaceFile(
	t *testing.T,
	h *harness,
	workspaceID string,
	relPath string,
) {
	t.Helper()
	var ws struct {
		WorktreePath string `json:"worktreePath"`
	}
	h.get("/v0/workspaces/"+workspaceID, &ws)
	require.NotEmpty(t, ws.WorktreePath)

	target := filepath.Join(ws.WorktreePath, relPath)
	f, err := os.OpenFile(target, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("\nregression-edit\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

type regressionStatusFile struct {
	Path   string `json:"path"`
	Staged bool   `json:"staged"`
}

func gitStatusFiles(
	t *testing.T,
	h *harness,
	base string,
) []regressionStatusFile {
	t.Helper()
	var status struct {
		Files []regressionStatusFile `json:"files"`
	}
	h.get(base+"/git/status", &status)
	return status.Files
}

func fileStaged(
	files []regressionStatusFile,
	path string,
) bool {
	for _, f := range files {
		if f.Path == path && f.Staged {
			return true
		}
	}
	return false
}
