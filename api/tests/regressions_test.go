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

// The repo home (worktreePath == repo.Path) is adopted as the IsDefault
// workspace, and that flag must survive persistence and reach the wire on GET
// /workspaces. The frontend pulls this workspace out of the sidebar tree and
// opens it from the repo header by its real id; if the DTO does not carry
// isDefault the default folder would render as a duplicate tree row. Under the
// workspace model the home STAYS on its protected default branch ("main") — it is
// no longer force-detached (spec §3.4) — and that same branch is surfaced as a
// separate, non-default PLACEHOLDER row held by the repo home.
func TestRegression_RepoHomeServedWithIsDefault(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	var workspaces []struct {
		ID         string `json:"id"`
		Branch     string `json:"branch"`
		IsDefault  bool   `json:"isDefault"`
		HeldByPath string `json:"heldByPath"`
	}
	h.get(repoBase+"/workspaces", &workspaces)

	var defaults []string // branches of the isDefault workspaces
	var placeholderHeld bool
	for _, w := range workspaces {
		if w.IsDefault {
			defaults = append(defaults, w.Branch)
		}
		// The same protected branch the home sits on is surfaced as a separate,
		// non-default placeholder that records the repo home as the branch holder.
		if !w.IsDefault && w.Branch == "main" && w.HeldByPath != "" {
			placeholderHeld = true
		}
	}
	require.Len(t, defaults, 1,
		"exactly one workspace (the repo home) must be flagged isDefault")
	require.Equal(t, "main", defaults[0],
		"the default repo-home workspace stays on its protected branch (spec §3.4)")
	require.True(t, placeholderHeld,
		"the protected 'main' branch the home holds is surfaced as a non-default placeholder")
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

// §13 / spec §3.6+§3.8: deleting a workspace tombstones it under the
// asynx-alignment delete lifecycle. Delete is now a PURE Send that folds
// Status=deleted (persist-then-purge): the DELETE is accepted (202) and a
// status:"deleted" frame is broadcast off the store/hub projections. The physical
// worktree purge moved off the synchronous write path into the async, gated delete
// reactor (Task 8; end-to-end lifecycle validated by the crash/lifecycle
// integration suite). The old entity-scoped storages tree
// (<home>/projects/<P>/<R>/workspaces/<W>/storages) no longer exists — central
// per-type event/read stores replace it — so there is no per-workspace dir to
// rm -rf here.
func TestRegression_DeleteWorkspaceTombstones(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// The read model is now eventually consistent (Send, not SendWait): wait until
	// the imported workspace is listable before deleting, so the background delete
	// cascade (which lists to build the tree) sees it.
	require.Eventually(t, func() bool {
		var rows []map[string]any
		h.get(repoBase+"/workspaces", &rows)
		for _, r := range rows {
			if r["id"] == imported.workspaceID {
				return true
			}
		}
		return false
	}, 5*time.Second, 25*time.Millisecond)

	conn := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodDelete, repoBase+"/workspaces/"+imported.workspaceID, nil,
		http.StatusAccepted)
	_ = resp.Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["status"] == "deleted"
	})
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

// workspaceWorktreePath resolves the on-disk worktree of a managed workspace.
// WorktreePath is no longer carried on the wire WorkspaceDTO (D13) and the daemon
// moved worktrees off the retired UUID layout to the human-readable path (spec
// §3.9), so it is reconstructed as <home>/projects/<P>/<slug>/<branch>, where the
// slug degrades to the repo's on-disk name (filepath.Base(repoPath)) for a
// no-remote fixture repo and the branch — a nested branch maps to nested
// directories — is read back from the workspace DTO.
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
		filepath.Base(imported.repoPath),
		workspaceBranch(t, h, imported),
	)
}

// workspaceBranch reads a managed workspace's branch back from its DTO. The wire
// no longer carries worktreePath, but branch is authoritative for the friendly
// worktree path (spec §3.9).
func workspaceBranch(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	var ws workspaceDTO
	h.get("/v0/projects/"+imported.projectID+"/repos/"+imported.repoID+
		"/workspaces/"+imported.workspaceID, &ws)
	return ws.Branch
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

// BUG-011: a repo folder that also contains one of its linked worktrees (git
// worktree add) must import as exactly ONE repo, and a linked worktree on a
// NON-protected branch (feature/linked) must NOT be auto-adopted — import only
// adopts the repo home plus a managed worktree per protected branch. POST
// /v0/projects creates the project; the repo is added explicitly via ImportRepo.
// Async: 202 + WS.
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
	addResp := h.raw(http.MethodPost, "/v0/projects/"+projectID+"/repos",
		map[string]string{"name": "repo", "path": repoPath}, http.StatusAccepted)
	_ = addResp.Body.Close()
	repo := readUntil(t, reposWS, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	repos := listRepos(t, h, projectID)
	require.Len(t, repos, 1,
		"a repo plus its linked worktree must discover as ONE repo")

	// Only the main worktree is auto-adopted. The linked worktree on a
	// NON-protected branch (feature/linked) is intentionally left for the user to
	// add explicitly — import must not flood the sidebar with every on-disk
	// checkout. Wait for the main adoption, then prove feature/linked never lands.
	workspacesWS := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == "main"
	})

	listURL := h.url + "/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces"
	countLinkedRaw := func() int {
		r, err := h.server.Client().Get(listURL)
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
			if w.Branch == "feature/linked" {
				n++
			}
		}
		return n
	}
	require.Never(t, func() bool { return countLinkedRaw() > 0 },
		2*time.Second, 100*time.Millisecond,
		"a non-protected linked worktree must not be auto-adopted at import")

	// Under the protected-branch model the repo home STAYS on its protected default
	// branch (main): the home is adopted as the single isDefault workspace, and that
	// same branch is surfaced once more as a locked, worktree-less PLACEHOLDER held by
	// the repo folder. The presence of a linked worktree must NOT duplicate either
	// row, so the invariant is exactly one isDefault "main" home + exactly one "main"
	// placeholder (not two homes, not a per-worktree fan-out).
	var workspaces []struct {
		Branch     string `json:"branch"`
		IsDefault  bool   `json:"isDefault"`
		HeldByPath string `json:"heldByPath"`
	}
	h.get("/v0/projects/"+projectID+"/repos/"+repoID+"/workspaces", &workspaces)

	var defaultMain, placeholderMain, linked int
	for _, ws := range workspaces {
		switch {
		case ws.Branch == "main" && ws.IsDefault:
			defaultMain++
		case ws.Branch == "main" && !ws.IsDefault && ws.HeldByPath != "":
			placeholderMain++
		case ws.Branch == "feature/linked":
			linked++
		}
	}
	require.Equal(t, 1, defaultMain,
		"the repo home (the main worktree) must register as exactly one isDefault workspace, not duplicated by the linked worktree")
	require.Equal(t, 1, placeholderMain,
		"the held main branch must surface as exactly one placeholder, not duplicated by the linked worktree")
	require.Equal(t, 0, linked,
		"the non-protected linked worktree must NOT be auto-adopted (user adds it explicitly)")
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

// TestRegression_DuplicateDefaultBranchWorkspace proves that creating a workspace
// on the repo's DEFAULT branch never persists a phantom duplicate. The default
// workspace (the imported repo folder) is unmanaged and does NOT count for the
// one-managed-workspace-per-branch guard, so the create is ACCEPTED (202) rather
// than falsely rejected — but git cannot check the already-checked-out default
// branch into a second worktree, so no duplicate row is ever created.
// Field bug: the sidebar once showed two "develop" rows; the duplicate pointed at
// the same main worktree with no distinct worktree of its own, so it could never
// be opened and only disappeared on reload.
// TestRegression_ImportDefaultBranchStaysHomeWithPlaceholder proves the fix for
// "Crowbar silently moved my checkout": the protected default branch held by the
// imported repo folder is NO LONGER force-detached. The repo home stays on the
// branch, and that same branch is surfaced as a non-default PLACEHOLDER row —
// locked, with no managed worktree on disk, recording the repo folder as the
// holder — until the user consents to free it (spec §3.4).
func TestRegression_ImportDefaultBranchStaysHomeWithPlaceholder(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// The repo home STAYS on `main`: Crowbar no longer moves the user's checkout
	// without consent.
	require.Equal(t, "main", currentBranch(t, imported.repoPath),
		"the repo home stays on the protected default branch — no silent force-detach")

	// The default branch is surfaced as a locked, non-default PLACEHOLDER: it has
	// no managed worktree on disk (empty localPath) and records the repo folder as
	// the holder (heldByPath).
	var workspaces []struct {
		Branch     string `json:"branch"`
		IsDefault  bool   `json:"isDefault"`
		Status     string `json:"status"`
		LocalPath  string `json:"localPath"`
		HeldByPath string `json:"heldByPath"`
	}
	h.get(repoBase+"/workspaces", &workspaces)

	var found bool
	for _, w := range workspaces {
		if w.Branch != "main" || w.IsDefault {
			continue
		}
		found = true
		require.Equal(t, "locked", w.Status, "the default-branch placeholder is locked")
		require.Empty(t, w.LocalPath,
			"a placeholder has no managed worktree, so it carries no localPath")
		require.True(t, samePathResolved(t, imported.repoPath, w.HeldByPath),
			"the placeholder records the repo folder as the branch holder, got %s", w.HeldByPath)
	}
	require.True(t, found,
		"the held default branch must be surfaced as a non-default placeholder row")
}

// currentBranch returns dir's checked-out branch, or "HEAD" when detached.
func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git rev-parse --abbrev-ref HEAD: %s", out)
	return strings.TrimSpace(string(out))
}

// dirExists reports whether path is an existing directory (goroutine-safe: no
// testing assertions, so it is callable from require.Eventually closures).
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
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
