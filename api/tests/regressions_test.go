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
	// Its arrival is the signal — block for it, no deadline.
	snapshot := readTextFrame(t, conn)
	require.NotContains(t, string(snapshot), `"files":null`,
		"snapshot frame must serialise files as [], not null")

	// The topic must now stay quiet while the tree is idle. "Quiet" is a
	// NEGATIVE, and a read deadline can only ever guess at one: it proves
	// "nothing showed up within 2s", which under load false-FAILS just as
	// readily as it false-passes. Close the idle window with a SENTINEL
	// instead — dirty a file of our own in the watched worktree and block (no
	// deadline) until the frame carrying it arrives.
	//
	// The sentinel travels the SAME topic, out of the same watcher, down the
	// same per-connection FIFO as any storm frame would. So once it lands, every
	// frame the idle period was ever going to produce has already been delivered
	// on this connection, and the frames counted BEFORE it are exactly the
	// idle-period frames — the negative, proven exactly, with no clock.
	//
	// Tolerate at most one straggler: the watcher does no recompute on subscribe
	// (the snapshot already carried fresh status), so its FIRST fan-out always
	// broadcasts, and only subsequent IDENTICAL frames are deduped. An
	// identical-frame stream is the bug.
	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, imported), sentinelFile, "quiet probe\n")
	defer stop()

	idle := 0
	for {
		var frame map[string]any
		require.NoError(t, json.Unmarshal(readTextFrame(t, conn), &frame))
		if statusHasFile(frame, sentinelFile) {
			return // sentinel landed: the idle window is closed, and it was quiet
		}
		idle++
		require.LessOrEqual(t, idle, 1,
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

	// Enough concurrent mutators to genuinely contend on the shared index.lock (the
	// point of the test) without spawning so many real git subprocesses that a
	// -race run's 10x slowdown starves the machine and the OS kills one mid-op (the
	// failure this exercises is lock contention, not fork/exec resource exhaustion).
	const (
		workers    = 4
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

	// The read model is eventually consistent (Send, not SendWait), so the import's
	// row can trail the call that made it. Quiesce is the deterministic
	// read-your-writes barrier — every projection folded — so the workspace is
	// listable when it returns, and the background delete cascade below (which lists
	// to build its tree) is guaranteed to see it. No polling, no window to miss.
	h.Quiesce()
	var rows []map[string]any
	h.get(repoBase+"/workspaces", &rows)
	require.Contains(t, workspaceIDs(rows), imported.workspaceID,
		"the imported workspace must be listable before the delete cascade runs")

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

// workspaceWorktreePath resolves the on-disk worktree of a managed workspace by
// reading back the path the PROVISIONER ITSELF persisted
// (domain.Workspace.WorktreePath) — the same ground truth kit.Env.WorktreePath
// serves the blackbox suites — rather than re-deriving it test-side.
//
// WorktreePath is deliberately server-side only and never carried on the wire
// WorkspaceDTO (spec §5/§8, D13), so the HTTP surface cannot answer this; but the
// harness boots the real app in-process, so the aggregate can. Workspace.Get
// folds the aggregate straight from the event log (§3.7) — always current, no
// read-model rebuild, no projection lag, so no barrier is needed.
//
// Reading it back is also the only way to be RIGHT. The previous version
// FABRICATED <home>/projects/<P>/<slug>/<branch> by string-joining, which silently
// dropped the trailing "worktree" leaf that worktreepath.Derive appends (spec
// §3.9). That shorter path is the workspace ROOT — the directory holding the git
// worktree and its sibling "chats" tree — not the worktree. The root EXISTS, so
// the mistake was invisible rather than loud: writes through it landed beside the
// git worktree instead of inside it (no git status change, no scoped file event),
// and reads of a tracked file 404'd in a directory that was perfectly real.
func workspaceWorktreePath(
	t *testing.T,
	h *harness,
	imported importedRepo,
) string {
	t.Helper()
	ws, err := h.app.Repositories.Workspace.Get(t.Context(), imported.workspaceID)
	require.NoError(t, err, "read back the provisioned worktree path")
	require.NotEmpty(t, ws.WorktreePath,
		"a managed workspace must carry the worktree path its provisioner used")
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

	// feature/linked must NEVER be adopted. Watching for two seconds and concluding
	// "it did not show up" proves only that it had not shown up YET — on a loaded
	// machine a slow importer makes that pass for the wrong reason, and it would keep
	// passing even if the adoption were merely late rather than absent.
	//
	// The sound form of a NEVER is: run the producer to COMPLETION, then assert once.
	// Quiesce drains the import's projections, so when it returns the importer has
	// finished deciding what to adopt and nothing is left in flight that could still
	// add the row. Its absence is then final.
	h.Quiesce()
	require.Zero(t, countBranchRows(t, h, projectID, repoID, "feature/linked"),
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

// TestRegression_DetachHolderFreesBranchAndMaterialisesWorktree proves the
// Detach-holder flow end-to-end through the real HTTP + git stack (spec
// §3.5/§3.7): a protected default branch the repo home still holds is surfaced as
// a placeholder; POST .../detach-holder moves the home to a detached HEAD
// (releasing the branch — the working tree is untouched) and re-provisions that
// branch into its OWN managed worktree in place. The placeholder becomes a real,
// on-disk locked worktree — heldByPath cleared, localPath set, .git present —
// with no LastError. The op is async (202 Accepted), so the outcome is observed
// by polling the read model. This is the deterministic contract that "Detach
// works" (it once failed silently when the derived path was already occupied).
func TestRegression_DetachHolderFreesBranchAndMaterialisesWorktree(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// Precondition: the home sits on main (Crowbar never force-detaches).
	require.Equal(t, "main", currentBranch(t, imported.repoPath),
		"precondition: repo home holds the protected default branch")

	type wsRow struct {
		ID         string `json:"id"`
		Branch     string `json:"branch"`
		IsDefault  bool   `json:"isDefault"`
		Status     string `json:"status"`
		LocalPath  string `json:"localPath"`
		HeldByPath string `json:"heldByPath"`
		LastError  string `json:"lastError"`
	}
	// Raw fetch: tolerates transport errors rather than asserting, so it stays usable
	// as a plain read after the async op has been drained to completion.
	listURL := h.url + repoBase + "/workspaces"
	fetchRows := func() []wsRow {
		r, err := h.server.Client().Get(listURL)
		if err != nil {
			return nil
		}
		defer func() { _ = r.Body.Close() }()
		var env struct {
			Data []wsRow `json:"data"`
		}
		if json.NewDecoder(r.Body).Decode(&env) != nil {
			return nil
		}
		return env.Data
	}

	var placeholderID string
	for _, w := range fetchRows() {
		if w.Branch == "main" && !w.IsDefault {
			placeholderID = w.ID
			require.Empty(t, w.LocalPath, "precondition: placeholder has no managed worktree yet")
			require.NotEmpty(t, w.HeldByPath, "precondition: placeholder records the repo home as the holder")
		}
	}
	require.NotEmpty(t, placeholderID, "the held main branch must surface as a placeholder")

	// Act: detach the holder (async → 202 Accepted). Dial BEFORE the POST so the
	// working overlay's rising edge can never be missed.
	conn := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodPost, repoBase+"/workspaces/"+placeholderID+"/detach-holder", nil, http.StatusAccepted)
	_ = resp.Body.Close()

	// The 202 only means "accepted": the real work — freeing the branch from the repo
	// home and running `git worktree add` to materialise the placeholder — happens in a
	// detached goroutine. The daemon brackets that goroutine with its working overlay,
	// so the falling edge is the op ANNOUNCING it is finished. Block on that, then fold
	// the projections, and the read model can be inspected as a settled fact instead of
	// polled for 10 seconds.
	waitForWorkComplete(t, conn, placeholderID)
	h.QuiesceReactors()

	// Assert: the placeholder became a real, on-disk managed worktree — the branch was
	// freed and re-provisioned in place, with no error.
	var final wsRow
	for _, w := range fetchRows() {
		if w.ID == placeholderID {
			final = w
		}
	}
	require.Equal(t, placeholderID, final.ID, "the placeholder row must still exist after detach-holder")
	require.Empty(t, final.HeldByPath, "detach-holder must free the branch from the repo home")
	require.Empty(t, final.LastError, "detach-holder must materialise the placeholder with no error")
	require.NotEmpty(t, final.LocalPath, "detach-holder must give the placeholder a managed worktree")
	require.True(t, dirExists(final.LocalPath), "the materialised worktree must exist on disk")

	require.FileExists(t, filepath.Join(final.LocalPath, ".git"),
		"the materialised placeholder must be a real linked git worktree (.git pointer present)")
	require.Equal(t, "locked", final.Status,
		"a protected-branch worktree stays locked after materialisation")

	// And: the repo home was moved to a detached HEAD — the branch is genuinely
	// released, not merely reported as freed.
	require.Equal(t, "HEAD", currentBranch(t, imported.repoPath),
		"detach-holder leaves the repo home on a detached HEAD, releasing the branch")
}

// currentBranch returns dir's checked-out branch, or "HEAD" when detached.
func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git rev-parse --abbrev-ref HEAD: %s", out)
	return strings.TrimSpace(string(out))
}

// dirExists reports whether path is an existing directory. It makes no testing
// assertions, so it composes freely inside other predicates.
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

	// Create a child workspace on a fresh branch (async 202). The row is not produced
	// by the request at all — it is produced by the detached provisioner the 202 spawns,
	// which no projection drain can see (a drain can only settle work that has already
	// been dispatched; here the dispatch itself is what we are waiting for).
	//
	// The workspace's own broadcast IS the signal: the aggregate announces the new row
	// on the repo-scoped stream the moment it exists. Dial BEFORE the POST so it cannot
	// be missed, block on the branch appearing, then fold the list projection the
	// assertion reads through.
	conn := h.dial(base + "/workspaces")
	_ = h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/dup"}, http.StatusAccepted).Body.Close()
	readUntil(t, conn, func(m map[string]any) bool { return m["branch"] == "feature/dup" })
	h.Quiesce()
	require.Equal(t, 1, countBranchRows(t, h, imported.projectID, imported.repoID, "feature/dup"),
		"the first feature/dup workspace must land")

	// A second create on the same branch is rejected synchronously with 409.
	resp := h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/dup"}, http.StatusConflict)
	_ = resp.Body.Close()

	// And never produces a duplicate. The 409 is SYNCHRONOUS — the request was refused
	// on the write path and scheduled no work at all — so there is nothing in flight to
	// wait out; the 2-second Never here was watching a producer that had already been
	// told no. Drain anyway (cheap, and it forecloses the theory that something was
	// still settling), then assert the count once.
	h.QuiesceReactors()
	require.Equal(t, 1, countBranchRows(t, h, imported.projectID, imported.repoID, "feature/dup"),
		"a rejected duplicate must never persist a second feature/dup workspace")
}

// countBranchRows returns how many workspaces in the repo's read model sit on the
// given branch. It is the shared oracle for the one-per-branch invariants, read
// through the same list endpoint a client would use.
func countBranchRows(
	t *testing.T,
	h *harness,
	projectID string,
	repoID string,
	branch string,
) int {
	t.Helper()
	n := 0
	for _, w := range listWorkspaces(t, h, projectID, repoID) {
		if w.Branch == branch {
			n++
		}
	}
	return n
}

// workspaceIDs extracts the id of every row in a decoded workspace list.
func workspaceIDs(
	rows []map[string]any,
) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if id, ok := r["id"].(string); ok {
			out = append(out, id)
		}
	}
	return out
}

// BUG-STALE-BASE: the workspace sidebar diff summary (added/deleted) must be
// measured against the base branch's CURRENT tip, not the frozen fork point
// recorded when the branch was created. In the field, enhancement/performance —
// branched from a recent develop but whose recorded fork point had gone stale —
// showed +69k/-44k because the summary diffed against the stale fork point,
// counting every commit develop had gained since. This test reproduces that
// exact shape: a child forked from a base branch that then advances by a large
// commit, with the child rebased onto the new base while its fork point stays
// frozen. The recomputed summary must count only the child's OWN change, proving
// the diff base tracks the advancing base branch (its live merge-base) rather
// than the stale fork point.
func TestRegression_SidebarDiffTracksAdvancingBaseNotStaleForkPoint(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// Create a child off the managed "main" worktree, WITH an explicit parent, so
	// its recorded fork point is main's tip at creation and its base branch is
	// resolvable as "main" (the shape the real bug had: parentId set).
	workspacesWS := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodPost, repoBase+"/workspaces",
		map[string]string{"branch": "feature/perf", "parentId": imported.workspaceID},
		http.StatusAccepted)
	_ = resp.Body.Close()
	created := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == "feature/perf" && m["status"] == "new"
	})
	childID, _ := created["id"].(string)
	require.NotEmpty(t, childID, "child create must broadcast an id")

	// Locate the managed worktrees on disk — they are linked worktrees of the
	// imported repo — so the test can drive real git against them.
	worktrees := worktreesByBranch(t, imported.repoPath)
	mainWorktree := worktrees["main"]
	childWorktree := worktrees["feature/perf"]
	require.NotEmpty(t, mainWorktree, "managed main worktree must exist")
	require.NotEmpty(t, childWorktree, "managed child worktree must exist")

	// The child makes one small change; the base branch (main) then advances by a
	// large commit the child does not yet have.
	require.NoError(t, writeFile(childWorktree, "child.txt", "child change\n"))
	runGit(t, childWorktree, "add", "child.txt")
	runGit(t, childWorktree, "commit", "-m", "child change")

	bigContent := strings.Repeat("base advancement line\n", 200)
	require.NoError(t, writeFile(mainWorktree, "big.txt", bigContent))
	runGit(t, mainWorktree, "add", "big.txt")
	runGit(t, mainWorktree, "commit", "-m", "advance base branch by 200 lines")

	// The child is rebased onto the advanced base: it now CONTAINS the 200-line
	// commit, while Crowbar's recorded fork point still points at the pre-advance
	// tip. A stale-fork summary would count all 200 lines; a base-branch summary
	// counts only the child's one line.
	runGit(t, childWorktree, "rebase", "main")

	// Recompute the summary (async) and wait for the work-complete edge.
	syncWS := h.dial(repoBase + "/workspaces")
	acc := h.raw(http.MethodPost, repoBase+"/workspaces/"+childID+"/sync", nil, http.StatusAccepted)
	_ = acc.Body.Close()
	waitForWorkComplete(t, syncWS, childID)

	var child workspaceDTO
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if w.ID == childID {
			child = w
		}
	}
	require.Equal(t, childID, child.ID, "child workspace must be in the list")
	require.Less(t, child.Added, 100,
		"summary must count only the child's own change, not the base branch's 200-line advancement (stale fork point counted ~201)")
	require.Equal(t, 0, child.Deleted, "the child deleted nothing relative to the advanced base")
}

// BUG-PULL-NO-CASCADE: pulling a PARENT branch (the develop-style base a stack of
// features is built on) must cascade a working-tree summary recompute to its CHILD
// workspaces. A child's sidebar diff (Added/Deleted) is measured against its parent
// branch's LIVE merge-base (workspace.summaryBase), so advancing the parent's tip
// via a pull silently STALES every child's badge until it is resynced. In the field
// the pull resynced ONLY the pulled workspace itself — leaving the identical gap
// worktree.finalizeMerge already closes for a merge (it resyncs BOTH parent and
// child after a merge moves a shared ref). This reproduces the shape: a child forked
// off a base branch whose own commit then lands on the base's remote; pulling the
// PARENT (never the child, never a manual /sync) must ALONE collapse the child's diff
// to +0/-0.
//
// The pullable parent here is an UNLOCKED "feature/base" branch, NOT literally
// "develop": the test's fallback provider marks main/develop/master protected, and a
// protected branch provisions LOCKED — which both rejects the pull write and never
// emits the status:"new" a create wait keys on. A non-protected feature branch is the
// faithful stand-in for a develop that a real git provider does NOT protect; the
// cascade mechanism under test is identical.
func TestRegression_PullingParentUpdatesChildDiffWithoutManualSync(t *testing.T) {
	h := newHarness(t)
	imported := importProjectWithOrigin(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	// Parent "feature/base": an UNLOCKED managed branch forked off the locked "main".
	// Push it to origin WITH upstream tracking so a bare `git pull` resolves a remote
	// to merge.
	baseID := createChildWorkspace(t, h, repoBase, "feature/base", imported.workspaceID)
	worktrees := worktreesByBranch(t, imported.repoPath)
	baseWorktree := worktrees["feature/base"]
	require.NotEmpty(t, baseWorktree, "managed base worktree must exist")
	runGit(t, baseWorktree, "push", "-u", "origin", "feature/base")

	// Child forked off feature/base, WITH parentId set, so its diff base resolves to
	// the parent branch "feature/base" (the shape the real bug had).
	childID := createChildWorkspace(t, h, repoBase, "feature/child", baseID)
	worktrees = worktreesByBranch(t, imported.repoPath)
	childWorktree := worktrees["feature/child"]
	require.NotEmpty(t, childWorktree, "managed child worktree must exist")

	// The child makes one committed change: +1 line vs "feature/base".
	require.NoError(t, writeFile(childWorktree, "child.txt", "child change\n"))
	runGit(t, childWorktree, "add", "child.txt")
	runGit(t, childWorktree, "commit", "-m", "child change")

	// Establish the STALE baseline: resync the child ONCE so the read model records
	// Added=1 BEFORE the pull. This is setup — not the behaviour under test — and the
	// assertion below proves the PULL alone (no post-pull child sync) refreshes it.
	syncBaseline(t, h, repoBase, childID)
	require.Equal(t, 1, childDiff(t, h, imported, childID).Added,
		"baseline: the child shows its own +1 against feature/base before the pull")

	// The child's commit lands on the base's remote (its PR is merged): push the child
	// branch's tip to origin's feature/base. Local "feature/base" is untouched, stale.
	runGit(t, childWorktree, "push", "origin", "feature/child:feature/base")

	// Pull the PARENT (feature/base) — the ONLY refresh action. Its ref fast-forwards
	// to include the child's commit, so the child's live merge-base(feature/base, HEAD)
	// now equals the child's own tip and its diff must collapse to +0/-0. Nothing syncs
	// the child explicitly; only the pull's cascade can refresh it.
	pullWS := h.dial(repoBase + "/workspaces")
	pull := h.raw(http.MethodPost, repoBase+"/workspaces/"+baseID+"/git/pull", nil, http.StatusAccepted)
	_ = pull.Body.Close()
	waitForWorkComplete(t, pullWS, baseID)
	// waitForWorkComplete only proves the PARENT's pull finished. The child's
	// refresh is the CASCADE off that pull — a post-commit reactor in its own
	// goroutine — so the parent going idle says nothing about whether the child
	// has been resynced yet. Join the reactors before reading the child, which
	// is precisely what QuiesceReactors exists for; without it this asserts on
	// the pre-cascade read model and sees the stale Added=1.
	h.QuiesceReactors()

	child := childDiff(t, h, imported, childID)
	require.Equal(t, childID, child.ID, "child workspace must be in the list")
	require.Equal(t, 0, child.Added,
		"pulling the parent must cascade a resync to the child so its diff reflects the advanced base (no manual child sync)")
	require.Equal(t, 0, child.Deleted, "the child's change is now fully contained in the pulled base")
}

// createChildWorkspace POSTs a child workspace on branch off parentID and returns
// its id, learned from the status:"new" WorkspaceDTO on the repo-scoped stream
// (dial-before-POST), mirroring importWritableWorkspace.
func createChildWorkspace(
	t *testing.T,
	h *harness,
	repoBase string,
	branch string,
	parentID string,
) string {
	t.Helper()
	workspacesWS := h.dial(repoBase + "/workspaces")
	resp := h.raw(http.MethodPost, repoBase+"/workspaces",
		map[string]string{"branch": branch, "parentId": parentID}, http.StatusAccepted)
	_ = resp.Body.Close()
	created := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == branch && m["status"] == "new"
	})
	id, _ := created["id"].(string)
	require.NotEmpty(t, id, "child create must broadcast an id for %q", branch)
	return id
}

// syncBaseline hits the manual /sync endpoint once and waits for work-complete, so
// a workspace's read-model summary reflects its current worktree BEFORE the action
// under test. Used only to set up a pre-condition, never as the propagation step.
func syncBaseline(
	t *testing.T,
	h *harness,
	repoBase string,
	wsID string,
) {
	t.Helper()
	syncWS := h.dial(repoBase + "/workspaces")
	acc := h.raw(http.MethodPost, repoBase+"/workspaces/"+wsID+"/sync", nil, http.StatusAccepted)
	_ = acc.Body.Close()
	waitForWorkComplete(t, syncWS, wsID)
}

// childDiff returns the workspace DTO for wsID from the repo's workspace list.
func childDiff(
	t *testing.T,
	h *harness,
	imported importedRepo,
	wsID string,
) workspaceDTO {
	t.Helper()
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if w.ID == wsID {
			return w
		}
	}
	return workspaceDTO{}
}

// importProjectWithOrigin imports a repo wired to a real bare origin remote (so
// git fetch/pull have somewhere to sync from), otherwise identical to
// importProject: the home is detached before import so the protected default
// branch "main" provisions as a single managed locked worktree (the returned
// workspaceID). The extra bare origin is what lets a child branch be pushed to the
// remote and a parent branch be pulled through the real HTTP endpoint.
func importProjectWithOrigin(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)
	// Wire a bare origin and push "main" WITH upstream tracking; the config lives in
	// the repo's shared .git/config, so every managed linked worktree inherits it and
	// a bare `git pull` on a tracked branch resolves its remote.
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare", "-b", "main")
	runGit(t, repoPath, "remote", "add", "origin", bare)
	runGit(t, repoPath, "push", "-u", "origin", "main")

	// Detach the home off "main" so the protected default branch is free and
	// provisions as its own managed locked worktree (mirrors importProject).
	runGit(t, repoPath, "checkout", "--detach")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	workspacesWS := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	adopted := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == "main"
	})
	wsID, _ := adopted["id"].(string)
	require.NotEmpty(t, wsID, "import must provision and broadcast the main managed worktree")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: wsID,
		repoPath:    repoPath,
	}
}

// worktreesByBranch maps each of repoPath's linked worktrees' checked-out branch
// to its on-disk path, parsed from `git worktree list --porcelain`. Managed
// Crowbar worktrees are linked worktrees of the imported repo, so this resolves
// their filesystem locations for tests that drive real git against them.
func worktreesByBranch(
	t *testing.T,
	repoPath string,
) map[string]string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git worktree list: %s", out)

	byBranch := make(map[string]string)
	current := ""
	for _, line := range strings.Split(string(out), "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok {
			current = path
			continue
		}
		if ref, ok := strings.CutPrefix(line, "branch "); ok {
			byBranch[strings.TrimPrefix(ref, "refs/heads/")] = current
		}
	}
	return byBranch
}

// BUG (create-child-from-stale-local): creating a child workspace from a parent
// branch that is BEHIND origin AND checked out in a locked managed worktree must
// fork the new branch from origin's fresh parent tip, NOT the stale local ref.
//
// The field repro, seen repeatedly: a branch created through the Crowbar UI lands
// several commits behind origin/develop. Root cause — addWorktree fast-forwarded
// the parent via `git fetch origin <parent>:<parent>`, which git REFUSES whenever
// <parent> is checked out in any worktree; in Crowbar's model every protected
// branch (the default branch included) is permanently checked out in its own
// locked managed worktree, so that fast-forward deterministically failed, warned
// into the daemon log where nobody saw it, and creation forked from the stale
// local tip. The fix fetches the remote-tracking ref only (`git fetch origin
// <parent>`, always allowed) and forks from the resolved origin/<parent>.
//
// This test seeds a bare origin, imports a clone at c1 (main provisioned as a
// locked managed worktree that pins local main at c1 — it cannot be moved because
// it is checked out), advances origin to c3 behind the clone's back, then creates
// a child off main. Before the fix the child forks from local main (c1) and this
// FAILS; after the fix it forks from origin/main (c3).
func TestRegression_CreateChildForksFromOriginTipNotStaleLocal(t *testing.T) {
	h := newHarness(t)

	root := t.TempDir()

	// Bare "origin" with HEAD pinned to main so default-branch resolution
	// (symbolic-ref origin/HEAD) is deterministic on the clone.
	origin := filepath.Join(root, "origin.git")
	runGit(t, root, "init", "--bare", "-b", "main", origin)

	// Seed clone: publish main @ c1 to origin, and keep it around to advance
	// origin later WITHOUT touching the imported working repo.
	seed := filepath.Join(root, "seed")
	runGit(t, root, "clone", origin, seed)
	runGit(t, seed, "config", "user.email", "t@t.dev")
	runGit(t, seed, "config", "user.name", "t")
	runGit(t, seed, "checkout", "-b", "main")
	require.NoError(t, writeFile(seed, "README.md", "c1\n"))
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "c1")
	runGit(t, seed, "push", "-u", "origin", "main")

	// The working repo Crowbar imports: a clone at c1. Detached so the protected
	// default branch provisions as its own locked managed worktree (mirrors the
	// import flow); local main stays at c1, pinned by that checked-out worktree.
	repoPath := filepath.Join(root, "repo")
	runGit(t, root, "clone", origin, repoPath)
	runGit(t, repoPath, "config", "user.email", "t@t.dev")
	runGit(t, repoPath, "config", "user.name", "t")
	runGit(t, repoPath, "checkout", "--detach")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	// Main provisioning as a managed worktree is the import's completion signal.
	workspacesWS := h.dial("/v0/projects/" + projectID + "/repos/" + repoID + "/workspaces")
	mainWS := readUntil(t, workspacesWS, func(m map[string]any) bool {
		return m["branch"] == "main"
	})
	require.NotEmpty(t, mainWS["id"], "import must provision the main managed worktree")

	// Advance origin/main to c3 behind the imported repo's back: local main stays
	// at c1 (checked out, un-fast-forwardable) and the repo's origin/main
	// remote-tracking ref stays stale until a create fetches it.
	require.NoError(t, writeFile(seed, "README.md", "c2\n"))
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "c2")
	require.NoError(t, writeFile(seed, "README.md", "c3\n"))
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "c3")
	runGit(t, seed, "push", "origin", "main")
	originTip := strings.TrimSpace(runGitOut(t, seed, "rev-parse", "HEAD"))

	staleLocalTip := strings.TrimSpace(runGitOut(t, repoPath, "rev-parse", "refs/heads/main"))
	require.NotEqual(t, originTip, staleLocalTip,
		"precondition: local main must be stale relative to origin/main")

	// Create a child off main (no parentId → parent is the repo default branch).
	base := "/v0/projects/" + projectID + "/repos/" + repoID
	conn := h.dial(base + "/workspaces")
	_ = h.raw(http.MethodPost, base+"/workspaces",
		map[string]string{"branch": "feature/from-origin-tip"}, http.StatusAccepted).Body.Close()
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["branch"] == "feature/from-origin-tip" && m["status"] == "new"
	})
	childID, _ := created["id"].(string)
	require.NotEmpty(t, childID, "child workspace create must broadcast an id")

	// Read the child worktree the provisioner recorded and check its fork point.
	childWS, err := h.app.Repositories.Workspace.Get(t.Context(), childID)
	require.NoError(t, err, "read back the provisioned child worktree path")
	require.NotEmpty(t, childWS.WorktreePath, "child must carry a worktree path")
	childHead := strings.TrimSpace(runGitOut(t, childWS.WorktreePath, "rev-parse", "HEAD"))

	require.Equal(t, originTip, childHead,
		"child of a stale, checked-out parent must fork from origin's fresh tip (%s), not the stale local ref (%s)",
		originTip, staleLocalTip)
}
