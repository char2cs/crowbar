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

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

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

	resp := h.raw(http.MethodGet, base+"/files", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

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
		repoBase + "/chats",
		repoBase + "/chats/" + imported.chatID,
		base + "/files/tree",
		docURL(imported.workspaceID, "README.md"),
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

func TestRegression_RepoHomeServedWithIsDefault(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h)

	workspaces := listWorkspaces(t, h, imported.projectID, imported.repoID)

	var defaults []string
	var placeholderHeld bool
	for _, w := range workspaces {
		if w.IsDefault {
			defaults = append(defaults, w.Branch)
		}
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

func TestRegression_GitTopicQuietWhenIdle(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)

	scope := "git:" + imported.workspaceID
	c := dialStream(t, h)
	c.Sub(t, scope, 0)

	snapshot := c.Next(t, func(f stream.Frame) bool { return f.T == stream.FrameSignal && f.Scope == scope })
	require.Contains(t, string(snapshot.Data), `"files":[]`,
		"snapshot signal must serialise files as [], not null")

	stop := rewriteOnTicker(t, workspaceWorktreePath(t, h, imported), sentinelFile, "quiet probe\n")
	defer stop()

	idle := 0
	c.Next(t, func(f stream.Frame) bool {
		if f.T != stream.FrameSignal {
			return false
		}
		var frame map[string]any
		require.NoError(t, json.Unmarshal(f.Data, &frame))
		if statusHasFile(frame, sentinelFile) {
			return true
		}
		idle++
		require.LessOrEqual(t, idle, 1,
			"idle workspace must not stream repeated git status frames")
		return false
	})
}

func TestRegression_GitMutationsSurviveLockContention(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	base := wsBase(imported)
	worktree := workspaceWorktreePath(t, h, imported)

	held := dialStream(t, h)
	held.Sub(t, "git:"+imported.workspaceID, 0)
	conn := held.conn
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

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

func TestRegression_WorkspaceDeleteBroadcastsDeletedStatus(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	feed := recordSidebar(t, h)
	resp := h.raw(http.MethodDelete, repoBase+"/chats/"+imported.chatID, nil,
		http.StatusOK)
	_ = resp.Body.Close()

	feed.awaitGone(stream.KindWorkspace, imported.workspaceID)
}

func TestRegression_DeleteLockedWorkspaceRejected(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	ws := worktreeOf(t, h, imported, imported.workspaceID)
	require.Equal(t, "locked", ws.Status, "adopted main worktree must be locked")

	msg := h.mutationError(http.MethodDelete, repoBase+"/chats/"+imported.chatID, nil,
		http.StatusConflict)
	require.NotEmpty(t, msg, "a locked delete must be refused with a reason")

	still := worktreeOf(t, h, imported, imported.workspaceID)
	require.Equal(t, imported.workspaceID, still.ID,
		"rejected delete must leave the workspace in place")

	resp := h.raw(http.MethodDelete, repoBase+"/chats/no-such-chat", nil,
		http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestRegression_DeleteWorkspaceTombstones(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	h.Quiesce()
	var found bool
	for _, w := range listWorkspaces(t, h, imported.projectID, imported.repoID) {
		if w.ID == imported.workspaceID {
			found = true
		}
	}
	require.True(t, found,
		"the imported workspace must be listable before the delete cascade runs")

	feed := recordSidebar(t, h)
	resp := h.raw(http.MethodDelete, repoBase+"/chats/"+imported.chatID, nil,
		http.StatusOK)
	_ = resp.Body.Close()
	feed.awaitGone(stream.KindWorkspace, imported.workspaceID)
}

func TestRegression_IconServedFromDiskNotGitHub(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	iconPath := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID + "/icon"

	resp := h.raw(http.MethodGet, iconPath, nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

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

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "wt-demo", "path": root}, http.StatusAccepted)
	_ = resp.Body.Close()
	project := projectsWS.read(t, func(m map[string]any) bool {
		return m["path"] == root
	})
	projectID, _ := project["id"].(string)
	require.NotEmpty(t, projectID)

	reposWS := watchEntities(t, h, stream.KindRepo)
	importRepo(t, h, "/v0/projects/"+projectID+"/repos", map[string]string{"name": "repo", "path": repoPath})
	repo := reposWS.read(t, func(m map[string]any) bool {
		return m["projectId"] == projectID
	})
	repoID, _ := repo["id"].(string)
	require.NotEmpty(t, repoID)

	repos := listRepos(t, h, projectID)
	require.Len(t, repos, 1,
		"a repo plus its linked worktree must discover as ONE repo")

	h.Quiesce()
	require.Zero(t, countBranchRows(t, h, projectID, repoID, "feature/linked"),
		"a non-protected linked worktree must not be auto-adopted at import")

	workspaces := listWorkspaces(t, h, projectID, repoID)

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

func TestRegression_BogusWorkspaceReadsAre404(t *testing.T) {
	h := newHarness(t)

	base := "/v0/chats/no-such-chat"
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

func TestRegression_EmptyPathParamsRejected(t *testing.T) {
	h := newHarness(t)

	paths := []string{
		"/v0/chats//git/status",
		"/v0/projects/p/repos//chats/c",
		"/v0/projects//repos/r/chats/c",
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

func TestRegression_ImportDefaultBranchStaysHomeWithPlaceholder(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h)

	require.Equal(t, "main", currentBranch(t, imported.repoPath),
		"the repo home stays on the protected default branch — no silent force-detach")

	workspaces := listWorkspaces(t, h, imported.projectID, imported.repoID)

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

func TestRegression_DetachHolderFreesBranchAndMaterialisesWorktree(t *testing.T) {
	h := newHarness(t)
	imported := importProjectHomeHoldsDefault(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	require.Equal(t, "main", currentBranch(t, imported.repoPath),
		"precondition: repo home holds the protected default branch")

	type wsRow struct {
		ID         string
		Branch     string
		IsDefault  bool
		Status     string
		LocalPath  string
		HeldByPath string
	}
	listURL := h.url + repoBase + "/chats"
	fetchRows := func() []wsRow {
		r, err := h.server.Client().Get(listURL)
		if err != nil {
			return nil
		}
		defer func() { _ = r.Body.Close() }()
		var env struct {
			Data []chatRow `json:"data"`
		}
		if json.NewDecoder(r.Body).Decode(&env) != nil {
			return nil
		}
		var rows []wsRow
		for _, c := range env.Data {
			if c.Worktree == nil {
				continue
			}
			rows = append(rows, wsRow{
				ID: c.WorkspaceID, Branch: c.Worktree.Branch, IsDefault: c.Worktree.IsDefault,
				Status: c.Worktree.Status, LocalPath: c.Worktree.LocalPath,
				HeldByPath: c.Worktree.HeldByPath,
			})
		}
		return rows
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

	placeholderChatID := owningChatID(t, h, placeholderID)
	detach := startOperation(t, h, repoBase+"/chats/"+placeholderChatID+"/detach-holder", nil)
	require.Equal(t, domain.OpSucceeded, awaitOperation(t, h, detach.ID, operationSettled).State)
	h.QuiesceReactors()

	var final wsRow
	for _, w := range fetchRows() {
		if w.ID == placeholderID {
			final = w
		}
	}
	require.Equal(t, placeholderID, final.ID, "the placeholder row must still exist after detach-holder")
	require.Empty(t, final.HeldByPath, "detach-holder must free the branch from the repo home")
	require.NotEmpty(t, final.LocalPath, "detach-holder must give the placeholder a managed worktree")
	require.True(t, dirExists(final.LocalPath), "the materialised worktree must exist on disk")

	require.FileExists(t, filepath.Join(final.LocalPath, ".git"),
		"the materialised placeholder must be a real linked git worktree (.git pointer present)")
	require.Equal(t, "locked", final.Status,
		"a protected-branch worktree stays locked after materialisation")

	require.Equal(t, "HEAD", currentBranch(t, imported.repoPath),
		"detach-holder leaves the repo home on a detached HEAD, releasing the branch")
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git rev-parse --abbrev-ref HEAD: %s", out)
	return strings.TrimSpace(string(out))
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

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

func TestRegression_SidebarDiffTracksAdvancingBaseNotStaleForkPoint(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	childID, childChatID := createWorktree(t, h, imported, "feature/perf", imported.workspaceID)

	worktrees := worktreesByBranch(t, imported.repoPath)
	mainWorktree := worktrees["main"]
	childWorktree := worktrees["feature/perf"]
	require.NotEmpty(t, mainWorktree, "managed main worktree must exist")
	require.NotEmpty(t, childWorktree, "managed child worktree must exist")

	require.NoError(t, writeFile(childWorktree, "child.txt", "child change\n"))
	runGit(t, childWorktree, "add", "child.txt")
	runGit(t, childWorktree, "commit", "-m", "child change")

	bigContent := strings.Repeat("base advancement line\n", 200)
	require.NoError(t, writeFile(mainWorktree, "big.txt", bigContent))
	runGit(t, mainWorktree, "add", "big.txt")
	runGit(t, mainWorktree, "commit", "-m", "advance base branch by 200 lines")

	runGit(t, childWorktree, "rebase", "main")

	sync := startOperation(t, h, repoBase+"/chats/"+childChatID+"/sync", nil)
	require.Equal(t, domain.OpSucceeded, awaitOperation(t, h, sync.ID, operationSettled).State)
	h.Quiesce()

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

func TestRegression_PullingParentUpdatesChildDiffWithoutManualSync(t *testing.T) {
	h := newHarness(t)
	imported := importProjectWithOrigin(t, h)

	baseID, baseChatID := createWorktree(t, h, imported, "feature/base", imported.workspaceID)
	worktrees := worktreesByBranch(t, imported.repoPath)
	baseWorktree := worktrees["feature/base"]
	require.NotEmpty(t, baseWorktree, "managed base worktree must exist")
	runGit(t, baseWorktree, "push", "-u", "origin", "feature/base")

	childID, childChatID := createWorktree(t, h, imported, "feature/child", baseID)
	worktrees = worktreesByBranch(t, imported.repoPath)
	childWorktree := worktrees["feature/child"]
	require.NotEmpty(t, childWorktree, "managed child worktree must exist")

	require.NoError(t, writeFile(childWorktree, "child.txt", "child change\n"))
	runGit(t, childWorktree, "add", "child.txt")
	runGit(t, childWorktree, "commit", "-m", "child change")

	syncBaseline(t, h, imported, childChatID)
	require.Equal(t, 1, childDiff(t, h, imported, childID).Added,
		"baseline: the child shows its own +1 against feature/base before the pull")

	runGit(t, childWorktree, "push", "origin", "feature/child:feature/base")

	pull := startOperation(t, h, "/v0/chats/"+baseChatID+"/git/pull", nil)
	require.Equal(t, domain.OpSucceeded, awaitOperation(t, h, pull.ID, operationSettled).State)
	h.QuiesceReactors()

	child := childDiff(t, h, imported, childID)
	require.Equal(t, childID, child.ID, "child workspace must be in the list")
	require.Equal(t, 0, child.Added,
		"pulling the parent must cascade a resync to the child so its diff reflects the advanced base (no manual child sync)")
	require.Equal(t, 0, child.Deleted, "the child's change is now fully contained in the pulled base")
}

func syncBaseline(
	t *testing.T,
	h *harness,
	imported importedRepo,
	chatID string,
) {
	t.Helper()
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID
	sync := startOperation(t, h, repoBase+"/chats/"+chatID+"/sync", nil)
	require.Equal(t, domain.OpSucceeded, awaitOperation(t, h, sync.ID, operationSettled).State)
	h.Quiesce()
}

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

func importProjectWithOrigin(
	t *testing.T,
	h *harness,
) importedRepo {
	t.Helper()
	repoPath := gitRepoWithCommit(t)
	bare := t.TempDir()
	runGit(t, bare, "init", "--bare", "-b", "main")
	runGit(t, repoPath, "remote", "add", "origin", bare)
	runGit(t, repoPath, "push", "-u", "origin", "main")

	runGit(t, repoPath, "checkout", "--detach")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	h.Quiesce()
	var wsID, chatID string
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree != nil && c.Worktree.Branch == "main" && c.Worktree.HeldByPath == "" {
			wsID, chatID = c.WorkspaceID, c.ID
			break
		}
	}
	require.NotEmpty(t, wsID, "import must provision the main managed worktree")

	return importedRepo{
		projectID:   projectID,
		repoID:      repoID,
		workspaceID: wsID,
		chatID:      chatID,
		repoPath:    repoPath,
	}
}

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

func TestRegression_CreateChildForksFromOriginTipNotStaleLocal(t *testing.T) {
	h := newHarness(t)

	root := t.TempDir()

	origin := filepath.Join(root, "origin.git")
	runGit(t, root, "init", "--bare", "-b", "main", origin)

	seed := filepath.Join(root, "seed")
	runGit(t, root, "clone", origin, seed)
	runGit(t, seed, "config", "user.email", "t@t.dev")
	runGit(t, seed, "config", "user.name", "t")
	runGit(t, seed, "checkout", "-b", "main")
	require.NoError(t, writeFile(seed, "README.md", "c1\n"))
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "c1")
	runGit(t, seed, "push", "-u", "origin", "main")

	repoPath := filepath.Join(root, "repo")
	runGit(t, root, "clone", origin, repoPath)
	runGit(t, repoPath, "config", "user.email", "t@t.dev")
	runGit(t, repoPath, "config", "user.name", "t")
	runGit(t, repoPath, "checkout", "--detach")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)

	h.Quiesce()
	var sawMain bool
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree != nil && c.Worktree.Branch == "main" {
			sawMain = true
		}
	}
	require.True(t, sawMain, "import must provision the main managed worktree")

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

	childID := createChildWorkspace(t, h, importedRepo{projectID: projectID, repoID: repoID},
		"feature/from-origin-tip", "")
	require.NotEmpty(t, childID, "child workspace create must return an id")

	h.Quiesce()
	childWS, err := h.app.Repositories.Workspace.Get(t.Context(), childID)
	require.NoError(t, err, "read back the provisioned child worktree path")
	require.NotEmpty(t, childWS.WorktreePath, "child must carry a worktree path")
	childHead := strings.TrimSpace(runGitOut(t, childWS.WorktreePath, "rev-parse", "HEAD"))

	require.Equal(t, originTip, childHead,
		"child of a stale, checked-out parent must fork from origin's fresh tip (%s), not the stale local ref (%s)",
		originTip, staleLocalTip)
}
