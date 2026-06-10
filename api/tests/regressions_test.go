//go:build integration

package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// P1 (UX QA 2026-06-10): git mutations once 500'd intermittently with
// "Unable to create '<repo>/.git/worktrees/<wt>/index.lock': File exists"
// because the fs watcher's `git status` reads opportunistically took the index
// lock on every shared-.git ref event and raced user mutations for it. Reads
// now run with GIT_OPTIONAL_LOCKS=0 and mutations retry briefly on a held
// lock. This test hammers stage/unstage/discard from many goroutines, dirtying
// files between calls so the live watcher keeps recomputing status, and
// asserts no request ever surfaces the index.lock failure.
func TestRegression_GitMutationsSurviveLockContention(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := "/v0/workspaces/" + imported.workspaceID
	worktree := workspaceWorktreePath(t, h, imported.workspaceID)

	// Subscribing to the git topic keeps the workspace watcher live so its
	// status reads contend with the mutations below. Drain frames so the
	// broadcaster never stalls on this connection.
	conn := h.dial("/v0/ws/git?wsId=" + imported.workspaceID)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	const (
		workers    = 10
		iterations = 5
	)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []string
	)
	report := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		errs = append(errs, fmt.Sprintf(format, args...))
	}
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("contend-%d.txt", id)
			for i := 0; i < iterations; i++ {
				content := fmt.Sprintf("worker %d iteration %d\n", id, i)
				if err := writeFile(worktree, name, content); err != nil {
					report("worker %d: write: %v", id, err)
					return
				}
				body := map[string]any{"paths": []string{name}}
				for _, action := range []string{"stage", "unstage", "discard"} {
					status, respBody := postJSONStatus(t, h, base+"/git/"+action, body)
					if status == http.StatusOK {
						continue
					}
					report("worker %d iter %d: %s -> %d: %s", id, i, action, status, respBody)
					return
				}
			}
		}(worker)
	}
	wg.Wait()
	require.Empty(t, errs, "git mutations must survive watcher lock contention:\n%s",
		strings.Join(errs, "\n"))
}

// P1 companion: a stale index.lock left behind by a killed backend (or killed
// git child) once wedged every mutation until it was removed by hand. A lock
// older than the engine's staleness threshold must now be cleared
// automatically and the mutation succeed.
func TestRegression_GitDiscardRecoversFromStaleIndexLock(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := "/v0/workspaces/" + imported.workspaceID
	worktree := workspaceWorktreePath(t, h, imported.workspaceID)

	dirtyWorkspaceFile(t, h, imported.workspaceID, "README.md")

	lock := filepath.Join(worktreeGitDir(t, worktree), "index.lock")
	require.NoError(t, os.WriteFile(lock, nil, 0o600))
	old := time.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(lock, old, old))

	h.post(base+"/git/discard", map[string]any{"paths": []string{"README.md"}}, http.StatusOK, nil)
	require.Empty(t, gitStatusFiles(t, h, base),
		"discard must succeed despite a stale index.lock")
	require.NoFileExists(t, lock, "the stale lock must have been cleared")
}

// workspaceWorktreePath resolves the on-disk worktree of a workspace via the API.
func workspaceWorktreePath(
	t *testing.T,
	h *harness,
	workspaceID string,
) string {
	t.Helper()
	var ws struct {
		WorktreePath string `json:"worktreePath"`
	}
	h.get("/v0/workspaces/"+workspaceID, &ws)
	require.NotEmpty(t, ws.WorktreePath)
	return ws.WorktreePath
}

// worktreeGitDir resolves the private git dir of a (possibly linked) worktree,
// i.e. where its index.lock lives.
func worktreeGitDir(
	t *testing.T,
	worktree string,
) string {
	t.Helper()
	cmd := exec.Command("git", "-C", worktree, "rev-parse", "--absolute-git-dir")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "rev-parse --absolute-git-dir: %s", out)
	return strings.TrimSpace(string(out))
}

// postJSONStatus is a goroutine-safe POST helper: unlike harness.post it never
// calls require, so concurrent workers can report failures themselves.
func postJSONStatus(
	t *testing.T,
	h *harness,
	path string,
	body any,
) (int, string) {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		return 0, err.Error()
	}
	req, err := http.NewRequest(http.MethodPost, h.url+path, bytes.NewReader(encoded))
	if err != nil {
		return 0, err.Error()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.server.Client().Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
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

// Empty path params: gin's radix tree matches /v0/workspaces//chats against
// /v0/workspaces/:wsId/chats with wsId == "" — the backend once answered such
// requests with 200 and data scoped to a nonexistent workspace. Every v0 route
// must reject an empty :wsId / :id segment with a 400 error envelope (enforced
// by the rejectEmptyPathParams middleware on the v0 group).
func TestRegression_EmptyPathParamsRejected(t *testing.T) {
	h := newHarness(t)

	paths := []string{
		"/v0/workspaces//chats",
		"/v0/workspaces//git/status",
	}
	for _, path := range paths {
		resp := h.raw(http.MethodGet, path, nil, http.StatusBadRequest)

		var env struct {
			Success bool            `json:"success"`
			Error   string          `json:"error"`
			Data    json.RawMessage `json:"data"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&env), "GET %s", path)
		_ = resp.Body.Close()

		require.False(t, env.Success, "GET %s must carry an error envelope", path)
		require.NotEmpty(t, env.Error, "GET %s error envelope must carry a message", path)
		require.Empty(t, env.Data, "GET %s error envelope must not carry data", path)
	}
}
