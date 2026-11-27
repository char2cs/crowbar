//go:build integration

package tests

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

type importFixture struct {
	origin    string
	seed      string
	repoPath  string
	projectID string
	repoID    string
	mainWSID  string
}

func newImportFixture(
	t *testing.T,
	h *harness,
) importFixture {
	t.Helper()
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
	var mainWSID string
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree != nil && c.Worktree.Branch == "main" && c.Worktree.HeldByPath == "" {
			mainWSID = c.WorkspaceID
			break
		}
	}
	require.NotEmpty(t, mainWSID, "import must provision main as a locked managed worktree")

	return importFixture{
		origin:    origin,
		seed:      seed,
		repoPath:  repoPath,
		projectID: projectID,
		repoID:    repoID,
		mainWSID:  mainWSID,
	}
}

func (f importFixture) repoBase() string {
	return "/v0/projects/" + f.projectID + "/repos/" + f.repoID
}

func (f importFixture) pushBranchToOrigin(
	t *testing.T,
	branch string,
	base string,
	content string,
) string {
	t.Helper()
	return f.pushFileToOrigin(t, branch, base, "f.txt", content)
}

func (f importFixture) pushFileToOrigin(
	t *testing.T,
	branch string,
	base string,
	file string,
	content string,
) string {
	t.Helper()
	runGit(t, f.seed, "checkout", "-B", branch, base)
	require.NoError(t, writeFile(f.seed, file, content))
	runGit(t, f.seed, "add", file)
	runGit(t, f.seed, "commit", "-m", "on "+branch)
	runGit(t, f.seed, "push", "-u", "origin", branch)
	sha := strings.TrimSpace(runGitOut(t, f.seed, "rev-parse", "HEAD"))
	runGit(t, f.seed, "checkout", "main")
	return sha
}

type branchEntry struct {
	Name         string `json:"name"`
	IsProtected  bool   `json:"isProtected"`
	HasWorkspace bool   `json:"hasWorkspace"`
}

func TestRegression_ImportBranchTakesRemoteContentNotDivergedLocal(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/diverged"
	originSha := f.pushBranchToOrigin(t, branch, "main", "REMOTE\n")

	runGit(t, f.repoPath, "fetch", "origin", branch+":"+branch)
	scratch := filepath.Join(t.TempDir(), "local")
	runGit(t, f.repoPath, "worktree", "add", scratch, branch)
	require.NoError(t, writeFile(scratch, "f.txt", "LOCAL\n"))
	runGit(t, scratch, "add", "f.txt")
	runGit(t, scratch, "commit", "-m", "local divergence")
	localSha := strings.TrimSpace(runGitOut(t, scratch, "rev-parse", "HEAD"))
	runGit(t, f.repoPath, "worktree", "remove", "--force", scratch)
	require.NotEqual(t, originSha, localSha,
		"precondition: the local branch must have diverged from origin")

	feed := recordSidebar(t, h)
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close()
	created := feed.awaitNewChat(hasWorkspace)
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID, "import must broadcast a workspace for the branch")

	h.Quiesce()
	ws, err := h.app.Repositories.Workspace.Get(t.Context(), wsID)
	require.NoError(t, err)
	require.NotEmpty(t, ws.WorktreePath,
		"the branch is free on disk, so it must materialise a worktree, not a placeholder")

	head := strings.TrimSpace(runGitOut(t, ws.WorktreePath, "rev-parse", "HEAD"))
	require.Equal(t, originSha, head,
		"an imported branch must check out origin's tip (%s), not the diverged local copy (%s)",
		originSha, localSha)
	require.Equal(t, originSha, ws.ForkPointSha,
		"the recorded fork point must be the commit actually checked out")
}

func TestRegression_ImportParentsUnderLockedDefaultBranchWorkspace(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/nests-under-main"
	f.pushBranchToOrigin(t, branch, "main", "work\n")

	feed := recordSidebar(t, h)
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close()
	feed.awaitNewChat(hasWorkspace)
	h.Quiesce()

	var parentID string
	found := false
	for _, w := range listWorkspaces(t, h, f.projectID, f.repoID) {
		if w.Branch == branch {
			parentID, found = w.ParentID, true
			break
		}
	}
	require.True(t, found, "import must produce a workspace for the branch")
	require.Equal(t, f.mainWSID, parentID,
		"an imported branch based on the default branch must nest under its locked workspace (%s), got %q",
		f.mainWSID, parentID)
}

func TestRegression_BranchListReflectsRemoteNotStaleTrackingRefs(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const fresh = "feature/pushed-after-import"
	f.pushBranchToOrigin(t, fresh, "main", "fresh\n")

	local := runGitOut(t, f.repoPath, "branch", "-r", "--format=%(refname:short)")
	require.NotContains(t, local, fresh,
		"precondition: the clone's remote-tracking refs must be stale")

	var entries []branchEntry
	h.get(f.repoBase()+"/branches", &entries)

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	require.Contains(t, names, fresh,
		"the branch list must fetch from origin first; got %v", names)

	runGit(t, f.seed, "push", "origin", "--delete", fresh)
	entries = nil
	h.get(f.repoBase()+"/branches", &entries)
	names = names[:0]
	for _, e := range entries {
		names = append(names, e.Name)
	}
	require.NotContains(t, names, fresh,
		"the branch list must prune deleted remote branches; got %v", names)
}

func TestRegression_RetryProvisionTakesRemoteContentNotDivergedLocal(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/held-then-retried"
	originSha := f.pushBranchToOrigin(t, branch, "main", "REMOTE\n")

	runGit(t, f.repoPath, "fetch", "origin", branch+":"+branch)
	work := filepath.Join(t.TempDir(), "work")
	runGit(t, f.repoPath, "worktree", "add", work, branch)
	require.NoError(t, writeFile(work, "f.txt", "LOCAL\n"))
	runGit(t, work, "add", "f.txt")
	runGit(t, work, "commit", "-m", "local divergence")
	localSha := strings.TrimSpace(runGitOut(t, work, "rev-parse", "HEAD"))
	require.NotEqual(t, originSha, localSha, "precondition: the local branch must have diverged")

	feed := recordSidebar(t, h)
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close()
	created := feed.awaitNewChat(hasWorkspace)
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID, "a branch held elsewhere must still produce a row")

	h.Quiesce()
	ws, err := h.app.Repositories.Workspace.Get(t.Context(), wsID)
	require.NoError(t, err)
	require.Empty(t, ws.WorktreePath, "a held branch must arrive as a placeholder")

	runGit(t, f.repoPath, "worktree", "remove", "--force", work)
	chatID := owningChatID(t, h, wsID)
	_ = h.raw(http.MethodPost,
		f.repoBase()+"/chats/"+chatID+"/retry-provision", nil, http.StatusAccepted).Body.Close()
	provisioned := feed.awaitWorkspace("provisioned", func(m map[string]any) bool {
		path, _ := m["localPath"].(string)
		return m["id"] == wsID && path != ""
	})
	path, _ := provisioned["localPath"].(string)
	require.NotEmpty(t, path, "retry must attach a worktree")

	head := strings.TrimSpace(runGitOut(t, path, "rev-parse", "HEAD"))
	require.Equal(t, originSha, head,
		"a retried import must check out origin's tip (%s), not the diverged local copy (%s)",
		originSha, localSha)
}

func TestRegression_ProvisionedWorktreeCanPullFromOrigin(t *testing.T) {
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
	runGit(t, seed, "checkout", "-b", "develop", "main")
	require.NoError(t, writeFile(seed, "d.txt", "d1\n"))
	runGit(t, seed, "add", "d.txt")
	runGit(t, seed, "commit", "-m", "d1")
	runGit(t, seed, "push", "-u", "origin", "develop")
	runGit(t, seed, "checkout", "main")

	repoPath := filepath.Join(root, "repo")
	runGit(t, root, "clone", origin, repoPath)
	runGit(t, repoPath, "config", "user.email", "t@t.dev")
	runGit(t, repoPath, "config", "user.name", "t")
	require.NotContains(t,
		runGitOut(t, repoPath, "branch", "--format=%(refname:short)"), "develop",
		"precondition: the clone must not already have a local develop")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)
	h.Quiesce()
	var path string
	for _, c := range listChats(t, h, projectID, repoID) {
		if c.Worktree != nil && c.Worktree.Branch == "develop" && c.Worktree.LocalPath != "" {
			path = c.Worktree.LocalPath
			break
		}
	}
	require.NotEmpty(t, path, "develop must provision a managed worktree")

	upstream := strings.TrimSpace(runGitOut(t, path, "rev-parse", "--abbrev-ref", "@{upstream}"))
	require.Equal(t, "origin/develop", upstream,
		"a worktree provisioned from origin must track origin/<branch>")

	head := strings.TrimSpace(runGitOut(t, path, "rev-parse", "HEAD"))
	runGit(t, seed, "checkout", "develop")
	require.NoError(t, writeFile(seed, "d.txt", "d2\n"))
	runGit(t, seed, "add", "d.txt")
	runGit(t, seed, "commit", "-m", "d2")
	runGit(t, seed, "push", "origin", "develop")
	originTip := strings.TrimSpace(runGitOut(t, seed, "rev-parse", "HEAD"))

	runGit(t, path, "pull", "--ff-only")
	pulled := strings.TrimSpace(runGitOut(t, path, "rev-parse", "HEAD"))
	require.NotEqual(t, head, pulled, "git pull must fast-forward the worktree onto origin's new commit")
	require.Equal(t, originTip, pulled, "the pulled tip must be origin's")
}

func TestRegression_ImportRejectsBranchMissingFromRemote(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	resp := h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches("feature/never-existed"), http.StatusBadRequest)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "feature/never-existed",
		"the refusal must name the branch so the client can surface it")
}

func TestRegression_AnImportedBranchChatPromptsOnTheDefaultProvider(t *testing.T) {
	h := newHarness(t)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude", ID: "importfake"})
	putProviderPrefs(t, h, providerPref{ID: fake.ID})
	f := newImportFixture(t, h)

	const branch = "feature/imported-chat"
	f.pushBranchToOrigin(t, branch, "main", "work\n")
	feed := recordSidebar(t, h)
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		importBranches(branch), http.StatusAccepted).Body.Close()
	created := feed.awaitNewChat(hasWorkspace)
	h.Quiesce()
	chatID, _ := created["id"].(string)
	require.NotEmpty(t, chatID)

	repo := importedRepo{projectID: f.projectID, repoID: f.repoID}
	require.Equal(t, fake.ID, getAgentChat(t, h, f.repoBase(), chatID).ActiveProviderID)
	require.Empty(t, fake.LaunchedRunners(t), "the import launches nothing")
	l := promptLaunch(t, h, repo, fake, chatID, "first words", "")
	assert.Contains(t, l.Argv, "first words")
	requireOneLaunch(t, fake, l)
}
