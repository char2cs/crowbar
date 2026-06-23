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
// loop (2026-06-10) plus the §13 contracts of the entity-scoped refactor. Each
// test names the bug or contract it guards against. If one of these fails, the
// frontend is broken in the corresponding way even when the rest of the suite
// passes.

// BUG-001: the file tree must be served at GET /files/tree. The backend once
// registered it at GET /files, which 404'd every file-explorer load while all
// other git/files routes kept working.
func TestRegression_FilesTreeServedAtTreePath(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := wsBase(imported)

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
// {success,error,data} envelope. The files/git/terminal groups once returned
// bare payloads, which the frontend's envelope-unwrapping fetch rejected
// wholesale — entire panels rendered empty with 200s on the wire. h.get fails
// the test unless the response carries a success envelope. The /chats and
// /v0/runs/running endpoints are gone (spec §12).
func TestRegression_AllReadEndpointsUseEnvelope(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := wsBase(imported)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	paths := []string{
		"/v0/projects",
		"/v0/projects/" + imported.projectID,
		"/v0/projects/" + imported.projectID + "/repos",
		repoBase,
		base,
		repoBase + "/workspaces",
		base + "/files/tree",
		base + "/files/content?path=README.md",
		base + "/git/status",
		base + "/git/log?limit=10&skip=0",
		base + "/git/branches",
		base + "/git/stashes",
		"/v0/settings/terminal/profiles",
	}
	for _, path := range paths {
		h.get(path, nil)
	}
}

// The imported repo's main worktree (worktreePath == repo.Path) is adopted as a
// workspace flagged IsDefault, and that flag must survive persistence and reach
// the wire on GET /workspaces. The frontend pulls this workspace out of the
// sidebar tree and opens it from the repo header by its real id; if the DTO does
// not carry isDefault the default folder would render as a duplicate tree row.
func TestRegression_MainWorktreeWorkspaceServedWithIsDefault(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	var workspaces []struct {
		ID        string `json:"id"`
		Branch    string `json:"branch"`
		IsDefault bool   `json:"isDefault"`
	}
	h.get(repoBase+"/workspaces", &workspaces)

	var defaultBranches []string
	var importedFlagged bool
	for _, w := range workspaces {
		if w.IsDefault {
			defaultBranches = append(defaultBranches, w.Branch)
		}
		if w.ID == imported.workspaceID {
			importedFlagged = w.IsDefault
		}
	}
	require.True(t, importedFlagged,
		"the adopted main-worktree workspace must be served with isDefault=true")
	require.Len(t, defaultBranches, 1,
		"exactly one workspace (the main worktree) must be flagged isDefault")
	require.Equal(t, "main", defaultBranches[0],
		"the default workspace must be the repo's main-worktree branch")
}

// BUG-010: git stage, unstage, and discard take {paths: []string} — including
// "." for everything — matching the frontend. The handlers once bound a
// singular {path}, so every stage click 400'd.
func TestRegression_StageUnstageDiscardAcceptPathsArray(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)

	dirtyWorkspaceFile(t, h, imported, "README.md")

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
	base := wsBase(imported)

	dirtyWorkspaceFile(t, h, imported, "README.md")
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

	resp := h.raw(http.MethodGet, wsBase(imported)+"/git/status", nil, http.StatusOK)
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

	conn := h.dial(wsBase(imported) + "/git/status")

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
	base := wsBase(imported)
	worktree := workspaceWorktreePath(t, h, imported)

	// Subscribing to the git topic keeps the workspace watcher live so its
	// status reads contend with the mutations below. Drain frames so the
	// broadcaster never stalls on this connection.
	conn := h.dial(base + "/git/status")
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
	base := wsBase(imported)
	worktree := workspaceWorktreePath(t, h, imported)

	dirtyWorkspaceFile(t, h, imported, "README.md")

	lock := filepath.Join(worktreeGitDir(t, worktree), "index.lock")
	require.NoError(t, os.WriteFile(lock, nil, 0o600))
	old := time.Now().Add(-time.Minute)
	require.NoError(t, os.Chtimes(lock, old, old))

	h.post(base+"/git/discard", map[string]any{"paths": []string{"README.md"}}, http.StatusOK, nil)
	require.Empty(t, gitStatusFiles(t, h, base),
		"discard must succeed despite a stale index.lock")
	require.NoFileExists(t, lock, "the stale lock must have been cleared")
}

// §13: workspace create flips from synchronous 201+body-id to 202+empty-body
// (spec §4). The created workspace must instead surface as a WorkspaceDTO on the
// repo-scoped Workspaces WS stream, first as status:"new". The id is learned
// from that frame — there is no id in a 202 body.
func TestRegression_WorkspaceCreateReturns202ThenWS(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	conn := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodPost, repoBase+"/workspaces",
		map[string]string{"branch": "feature/created-202"}, http.StatusAccepted)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	require.NotContains(t, string(body), `"id"`,
		"a 202 create must not carry an id in the body")

	got := readUntil(t, conn, func(m map[string]any) bool {
		return m["branch"] == "feature/created-202" && m["status"] == "new"
	})
	require.NotEmpty(t, got["id"], "the create must broadcast a WorkspaceDTO with an id")
}

// §13: deleting an unlocked workspace returns 202 and then broadcasts a
// status:"deleted" WorkspaceDTO tombstone on the Workspaces WS stream, so the
// client cache drops the entity (spec §4/§6).
func TestRegression_WorkspaceDeleteBroadcastsDeletedStatus(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	conn := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodDelete, repoBase+"/workspaces/"+imported.workspaceID, nil,
		http.StatusAccepted)
	_ = resp.Body.Close()

	got := readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["status"] == "deleted"
	})
	require.Equal(t, "deleted", got["status"])
}

// §13: the delete of a locked workspace is accepted (202) but the background
// cascade skips the locked row and surfaces the rejection as lastError on the
// Workspaces WS stream; the workspace remains. This replaces the old synchronous
// 409 contract while keeping the "locked rows are not silently dropped"
// behaviour coverage (BUG-009). The Locked bool was removed — the lock is now
// the status enum (spec §5).
func TestRegression_DeleteLockedWorkspaceRejected(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// The adopted main worktree is locked: "main" is a default protected branch.
	var ws workspaceDTO
	h.get(wsBase(imported), &ws)
	require.Equal(t, "locked", ws.Status, "adopted main worktree must be locked")

	conn := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodDelete, repoBase+"/workspaces/"+imported.workspaceID, nil,
		http.StatusAccepted)
	_ = resp.Body.Close()

	got := readUntil(t, conn, func(m map[string]any) bool {
		if m["id"] != imported.workspaceID {
			return false
		}
		le, _ := m["lastError"].(string)
		return le != ""
	})
	require.NotEmpty(t, got["lastError"], "a locked delete must surface lastError")

	// The workspace must still exist after the rejected delete.
	h.get(wsBase(imported), &ws)
	require.Equal(t, imported.workspaceID, ws.ID,
		"rejected delete must leave the workspace in place")

	// And an unknown id must 404 synchronously, never report a successful cascade.
	resp = h.raw(http.MethodDelete, repoBase+"/workspaces/no-such-workspace", nil,
		http.StatusNotFound)
	_ = resp.Body.Close()
}

// §13: deleting a workspace removes its entire per-workspace storage tree on
// disk (worktree + storages), not just the read-model row (spec §1/§8).
func TestRegression_DeleteWorkspaceRemovesStoragesDir(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	worktree := workspaceWorktreePath(t, h, imported)
	// The per-workspace dir is the parent of the .../worktree leaf.
	workspaceDir := filepath.Dir(worktree)
	require.DirExists(t, workspaceDir, "the workspace storage tree must exist before delete")

	conn := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodDelete, repoBase+"/workspaces/"+imported.workspaceID, nil,
		http.StatusAccepted)
	_ = resp.Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["status"] == "deleted"
	})

	require.NoDirExists(t, workspaceDir,
		"delete must rm -rf the whole per-workspace tree")
}

// §13: the repo icon is served from on-disk bytes, never proxied live from
// GitHub on the read path (spec §12). GET .../repos/:repoId/icon answers from
// the stored icon file; absent an on-disk icon it 404s rather than reaching out
// to a remote avatar. (The positive served-from-disk byte assertion lives in the
// kit blackbox suite, which controls the crowbar home dir.)
func TestRegression_IconServedFromDiskNotGitHub(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	iconPath := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID + "/icon"

	// With no on-disk icon (gh absent in the test env, so the import default
	// avatar fetch degrades to none) the endpoint must 404 — proving the read
	// path never falls through to a live GitHub fetch.
	resp := h.raw(http.MethodGet, iconPath, nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

// workspaceWorktreePath resolves the on-disk worktree of a workspace.
// WorktreePath is no longer carried on the wire WorkspaceDTO (D13), so it is
// reconstructed from the deterministic UUID layout the daemon uses:
// <home>/projects/<P>/<R>/workspaces/<W>/worktree.
func workspaceWorktreePath(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	return filepath.Join(
		h.home,
		"projects",
		imported.projectID,
		imported.repoID,
		"workspaces",
		imported.workspaceID,
		"worktree",
	)
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
	imported importedRepo,
	relPath string,
) {
	t.Helper()
	worktree := workspaceWorktreePath(t, h, imported)

	target := filepath.Join(worktree, relPath)
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

// BUG-005/BUG-006: importing a nonexistent path must fail with a clean 404
// error envelope (synchronous validation, before the 202 async path) and leave
// NO project behind. The import once persisted the project row before probing
// the path, so a typo'd import left a ghost, repo-less project in the sidebar.
func TestRegression_ImportNonexistentPathLeavesNoProject(t *testing.T) {
	h := newHarness(t)

	bogus := filepath.Join(t.TempDir(), "does-not-exist")
	h.postError("/v0/projects", map[string]string{"name": "ghost", "path": bogus},
		http.StatusBadRequest)

	var projects []struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects", &projects)
	require.Empty(t, projects,
		"a failed import must not leave a project behind")
}

// BUG-011: a project root containing a repo AND one of its linked worktrees
// (git worktree add) must import as exactly ONE repo. Discovery once treated
// the worktree's .git FILE (gitdir pointer) as a repo marker, registering the
// worktree as a second repo and adopting every worktree once per "repo" —
// duplicate workspaces all over the tree. Import is async (202 + WS).
func TestRegression_LinkedWorktreeImportsAsOneRepo(t *testing.T) {
	h := newHarness(t)

	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	require.NoError(t, os.Mkdir(repoPath, 0o755))
	runGit(t, repoPath, "init")
	runGit(t, repoPath, "config", "user.email", "t@t.dev")
	runGit(t, repoPath, "config", "user.name", "t")
	runGit(t, repoPath, "checkout", "-b", "main")
	require.NoError(t, writeFile(repoPath, "README.md", "hello\n"))
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "initial")
	runGit(t, repoPath, "worktree", "add", "-b", "feature/linked",
		filepath.Join(root, "wt"))

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "wt-demo", "path": root}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == root
	})
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := h.dial("/v0/projects/" + projectID + "/repos")
	repo := readUntil(t, reposWS, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	repos := listRepos(t, h, projectID)
	require.Len(t, repos, 1,
		"a repo plus its linked worktree must discover as ONE repo")

	// Both worktrees (main + linked) must each be adopted exactly once. Wait for
	// the second adoption on the workspaces WS, then read the persisted list.
	workspacesWS := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	seen := map[string]bool{}
	readUntil(t, workspacesWS, func(m map[string]any) bool {
		if b, ok := m["branch"].(string); ok {
			seen[b] = true
		}
		return seen["main"] && seen["feature/linked"]
	})

	workspaces := listWorkspaces(t, h, projectID, repoID)

	branches := map[string]int{}
	for _, ws := range workspaces {
		branches[ws.Branch]++
	}
	// The two on-disk worktrees (main + linked) must each be adopted exactly
	// once. Import additionally seeds locked stubs for the default protected
	// branches (develop, master) that have no worktree — those are not adoptions
	// and must not double-count the real worktrees.
	require.Equal(t, 1, branches["main"],
		"the main worktree must register as exactly one workspace")
	require.Equal(t, 1, branches["feature/linked"],
		"the linked worktree must register as exactly one workspace")
}

// BUG-008: reads scoped to a workspace id that does not exist must 404 with an
// error envelope. git/status and files/tree once 500'd on the not-found
// sentinel. The /chats surface is gone (spec §12).
func TestRegression_BogusWorkspaceReadsAre404(t *testing.T) {
	h := newHarness(t)

	base := "/v0/projects/no-such-project/repos/no-such-repo/workspaces/no-such-workspace"
	for _, path := range []string{
		base + "/git/status",
		base + "/files/tree",
	} {
		resp := h.raw(http.MethodGet, path, nil, http.StatusNotFound)

		var env struct {
			Success bool   `json:"success"`
			Error   string `json:"error"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&env), "GET %s", path)
		_ = resp.Body.Close()

		require.False(t, env.Success, "GET %s must carry an error envelope", path)
		require.NotEmpty(t, env.Error, "GET %s error envelope must carry a message", path)
	}
}

// Empty path params: gin's radix tree matches an empty path segment against a
// :wsId param. The backend once answered such requests with 200 and data scoped
// to a nonexistent workspace. Every v0 route must reject an empty
// :projectId/:repoId/:wsId segment with a 400 error envelope (enforced by the
// rejectEmptyPathParams middleware on the v0 group).
func TestRegression_EmptyPathParamsRejected(t *testing.T) {
	h := newHarness(t)

	paths := []string{
		"/v0/projects/p/repos/r/workspaces//git/status",
		"/v0/projects/p/repos//workspaces/w/git/status",
		"/v0/projects//repos/r/workspaces/w/git/status",
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

// TestRegression_DuplicateDefaultBranchWorkspace proves a create that would
// adopt the repo's main worktree a SECOND time (branch == the default branch,
// no parentId) is rejected and never persists a phantom duplicate workspace.
// Field bug: the sidebar showed two "develop" rows; the duplicate row pointed at
// the same main worktree with no distinct worktree of its own (git cannot check
// out an already-checked-out branch), so it could never be opened and only
// disappeared on reload. The fix guards adoptMainWorktree against re-adoption.
func TestRegression_DuplicateDefaultBranchWorkspace(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	countMain := func() int {
		n := 0
		for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
			if w.Branch == "main" {
				n++
			}
		}
		return n
	}
	require.Equal(t, 1, countMain(), "exactly one default (main) workspace after import")

	// Attempt to create a second workspace on the default branch with no parent —
	// the path that re-adopts the main worktree. The one-per-branch guard now
	// rejects this synchronously with 409 (empty body), so use raw rather than
	// the envelope-decoding post.
	_ = h.raw(http.MethodPost, base+"/workspaces", map[string]string{"branch": "main"}, http.StatusConflict).Body.Close()

	// The guard rejects it synchronously, but poll anyway to prove no duplicate
	// "main" workspace ever lands. countMainRaw drives a raw GET that tolerates
	// connection errors: require.Never runs its closure in a goroutine, where
	// h.get's require.NoError would FailNow from a non-test goroutine.
	mainURL := h.url + base + "/workspaces"
	countMainRaw := func() int {
		r, err := h.server.Client().Get(mainURL)
		if err != nil {
			return 0
		}
		defer func() { _ = r.Body.Close() }()
		var env struct {
			Data []workspaceDTO `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			return 0
		}
		n := 0
		for _, w := range env.Data {
			if w.Branch == "main" {
				n++
			}
		}
		return n
	}
	require.Never(t, func() bool { return countMainRaw() > 1 },
		2*time.Second, 100*time.Millisecond,
		"a duplicate default-branch workspace was persisted")
}

// TestRegression_DuplicateNonDefaultBranchWorkspace proves the one-per-branch
// invariant also covers child branches: once a workspace exists for a branch,
// creating a second workspace on that same branch is rejected (409 sync) and
// never persists a duplicate.
func TestRegression_DuplicateNonDefaultBranchWorkspace(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	base := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// Create a child workspace on a fresh branch (async 202).
	_ = h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/dup"}, http.StatusAccepted).Body.Close()

	countBranch := func(name string) int {
		n := 0
		for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
			if w.Branch == name {
				n++
			}
		}
		return n
	}
	require.Eventually(t, func() bool { return countBranch("feature/dup") == 1 },
		3*time.Second, 100*time.Millisecond, "first feature/dup workspace should land")

	// A second create on the same branch is rejected synchronously with 409.
	resp := h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/dup"}, http.StatusConflict)
	_ = resp.Body.Close()

	// And never produces a duplicate. countBranch is called in a goroutine by
	// require.Never, so drive it via a raw GET that tolerates connection errors
	// rather than require.NoError (which would FailNow from a non-test goroutine).
	dupURL := h.url + base + "/workspaces"
	countDupRaw := func() int {
		r, err := h.server.Client().Get(dupURL)
		if err != nil {
			return 0
		}
		defer func() { _ = r.Body.Close() }()
		var env struct {
			Data []workspaceDTO `json:"data"`
		}
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			return 0
		}
		n := 0
		for _, w := range env.Data {
			if w.Branch == "feature/dup" {
				n++
			}
		}
		return n
	}
	require.Never(t, func() bool { return countDupRaw() > 1 },
		2*time.Second, 100*time.Millisecond, "no duplicate feature/dup workspace")
}
