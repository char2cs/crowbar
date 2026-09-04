//go:build integration

package tests

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// This file pins the branch-import contract the import dialog depends on
// (spec §import): a branch imported from the picker must land as a managed
// workspace holding ORIGIN's content, parented under the locked workspace its
// PR (or the default branch) points at, and picked from a branch list that
// reflects the remote as it is NOW.

// importFixture is a bare origin plus the clone Crowbar imports, wired the way
// the field setup is: the clone's home is detached so the protected default
// branch "main" provisions as its own locked managed worktree.
type importFixture struct {
	origin    string
	seed      string
	repoPath  string
	projectID string
	repoID    string
	mainWSID  string
}

// newImportFixture seeds origin with main @ c1, clones it as the repo Crowbar
// imports, and waits for main's locked managed worktree — the import's
// completion signal. `seed` is a second clone used to publish branches to
// origin behind the imported repo's back, exactly as a teammate would.
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

	// Read back rather than awaited on a stream: the repo-scoped mount this
	// fixture would dial is created by the very ImportRepo job whose output it
	// wants, so by the time there is a repoId to dial the adoption has already
	// happened (see importProject's own doc comment for the same reasoning).
	// Quiesce is the deterministic barrier, not a poll.
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

// pushBranchToOrigin publishes branch @ a commit writing `content` to f.txt.
func (f importFixture) pushBranchToOrigin(
	t *testing.T,
	branch string,
	base string,
	content string,
) string {
	t.Helper()
	runGit(t, f.seed, "checkout", "-B", branch, base)
	require.NoError(t, writeFile(f.seed, "f.txt", content))
	runGit(t, f.seed, "add", "f.txt")
	runGit(t, f.seed, "commit", "-m", "on "+branch)
	runGit(t, f.seed, "push", "-u", "origin", branch)
	sha := strings.TrimSpace(runGitOut(t, f.seed, "rev-parse", "HEAD"))
	runGit(t, f.seed, "checkout", "main")
	return sha
}

// branchEntry mirrors the BranchEntry DTO the import dialog consumes.
type branchEntry struct {
	Name         string `json:"name"`
	IsProtected  bool   `json:"isProtected"`
	HasWorkspace bool   `json:"hasWorkspace"`
}

// BUG (import-takes-local-copy): importing a branch that ALSO exists locally in
// a diverged state must check out ORIGIN's commits, not the local ones.
//
// Root cause: checkoutRemoteBranch fast-forwarded via `git fetch origin <b>:<b>`,
// which git rejects as non-fast-forward whenever the local <b> has diverged
// (a stale local copy, or a force-push on the remote). The rejection was
// swallowed as a best-effort warning and `git worktree add <path> <b>` then
// checked out the LOCAL branch — so the user got their own old commits under a
// row that claims to be origin's branch, with a ForkPointSha pointing at a
// commit the worktree's history does not even contain.
func TestRegression_ImportBranchTakesRemoteContentNotDivergedLocal(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/diverged"
	originSha := f.pushBranchToOrigin(t, branch, "main", "REMOTE\n")

	// The imported repo picks the branch up, then diverges its LOCAL copy —
	// the everyday case of having worked on the branch before importing it.
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

	conn := h.dial(f.repoBase() + "/chats/ws")
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		map[string]any{"branches": []string{branch}}, http.StatusAccepted).Body.Close()
	// No worktree_state frame ever reaches a freshly imported chat: pushChatWorktree
	// drops it whenever the chat's SetWorkspace event has not yet reached the
	// AgentChat projection (owningChatIDFor resolves "" and the push is skipped),
	// which is exactly the moment a create races into. workspace_set is the
	// reliable signal instead — SpawnChatWithImportedWorktree only fires it once
	// CreateImportedWorkspace has already materialised the workspace, so its
	// WorkspaceID is the real one.
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["kind"] == "workspace_set"
	})
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID, "import must broadcast a workspace for the branch")

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

// BUG (import-parents-at-repo-root): a branch whose PR base is the DEFAULT
// branch — the overwhelmingly common case — must be parented under the default
// branch's LOCKED managed workspace, not dropped at the repo root.
//
// Root cause: resolveImportParent short-circuited `base == defaultBranch` to an
// empty ParentID (= the repo home) instead of looking the default branch up in
// the existing-workspace map like any other base. Every protected branch,
// including the default, gets its own locked managed workspace at repo import,
// so that lookup would have succeeded — imported branches simply never nested.
// The same short-circuit applies to a branch with no PR at all, which must also
// hang off the locked base branch.
func TestRegression_ImportParentsUnderLockedDefaultBranchWorkspace(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/nests-under-main"
	f.pushBranchToOrigin(t, branch, "main", "work\n")

	conn := h.dial(f.repoBase() + "/chats/ws")
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		map[string]any{"branches": []string{branch}}, http.StatusAccepted).Body.Close()
	// No worktree_state frame ever reaches a freshly imported chat (see the sibling
	// test's comment on the same wait), and the bare workspace_set event carries no
	// parentId at all — so this one reads back over REST once workspace_set proves
	// the import settled, rather than waiting on a frame that could never answer it.
	readUntil(t, conn, func(m map[string]any) bool {
		return m["kind"] == "workspace_set"
	})
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

// BUG (stale-import-branch-list): the import dialog must list the remote as it
// is NOW. GET …/branches read `git branch -r` with no fetch, and the only full
// `git fetch` in the daemon is the manual Git-panel action (OriginSyncManager
// refreshes a single subscribed protected branch via FetchRef). So a branch a
// teammate pushed after the repo was imported never appeared in the picker, and
// a branch deleted on the remote was offered forever.
func TestRegression_BranchListReflectsRemoteNotStaleTrackingRefs(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	// Published AFTER the import, so the clone's refs/remotes/origin/* has never
	// heard of it.
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

	// …and a branch deleted on the remote must stop being offered.
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

// BUG (retry-provision-takes-local-copy): Retry on a failed import must produce
// the SAME content a successful import would — origin's.
//
// materializeProtectedWorktree (RetryProvision, and DetachHolder through it)
// carried an identical copy of the checkoutRemoteBranch defect: fast-forward via
// `git fetch origin <b>:<b>`, swallow the non-fast-forward rejection, then
// `git worktree add <path> <b>` the LOCAL ref. So the fix for the import path
// alone still left "import → held → Detach…/Retry" — the recovery the placeholder
// row exists to offer — checking out the user's stale commits.
//
// The repro drives the real placeholder flow: an external worktree holds the
// branch, so the import can only produce a placeholder; the local branch is
// diverged; detaching the holder and retrying must land on origin's tip.
func TestRegression_RetryProvisionTakesRemoteContentNotDivergedLocal(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	const branch = "feature/held-then-retried"
	originSha := f.pushBranchToOrigin(t, branch, "main", "REMOTE\n")

	// A local copy that has diverged from origin…
	runGit(t, f.repoPath, "fetch", "origin", branch+":"+branch)
	work := filepath.Join(t.TempDir(), "work")
	runGit(t, f.repoPath, "worktree", "add", work, branch)
	require.NoError(t, writeFile(work, "f.txt", "LOCAL\n"))
	runGit(t, work, "add", "f.txt")
	runGit(t, work, "commit", "-m", "local divergence")
	localSha := strings.TrimSpace(runGitOut(t, work, "rev-parse", "HEAD"))
	require.NotEqual(t, originSha, localSha, "precondition: the local branch must have diverged")

	// …and it stays CHECKED OUT in that worktree, so the import cannot
	// materialise it and must fall back to a placeholder row.
	conn := h.dial(f.repoBase() + "/chats/ws")
	_ = h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		map[string]any{"branches": []string{branch}}, http.StatusAccepted).Body.Close()
	// No worktree_state frame ever reaches a freshly imported chat (see the sibling
	// tests' comment on the same wait); workspace_set is the reliable signal that
	// the placeholder row now exists, materialised or not.
	created := readUntil(t, conn, func(m map[string]any) bool {
		return m["kind"] == "workspace_set"
	})
	wsID, _ := created["workspaceId"].(string)
	require.NotEmpty(t, wsID, "a branch held elsewhere must still produce a row")

	ws, err := h.app.Repositories.Workspace.Get(t.Context(), wsID)
	require.NoError(t, err)
	require.Empty(t, ws.WorktreePath, "a held branch must arrive as a placeholder")

	// Free the branch and retry, exactly as the row's Retry action does.
	// retry-provision is now a chat-keyed lifecycle verb (spec §4.3): resolve the
	// chat owning the placeholder workspace rather than naming the workspace.
	runGit(t, f.repoPath, "worktree", "remove", "--force", work)
	chatID := owningChatID(t, h, wsID)
	_ = h.raw(http.MethodPost,
		f.repoBase()+"/chats/"+chatID+"/retry-provision", nil, http.StatusAccepted).Body.Close()
	provisioned := readUntilWorktree(t, conn, func(m map[string]any) bool {
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

// BUG (no-upstream-after-checkout-at-ref): every managed worktree Crowbar
// provisions from origin must be able to `git pull`.
//
// `git worktree add -B <branch> <sha>` — the checkout that makes an import
// authoritative about the remote — resolves a SHA, so unlike the plain
// `git worktree add <path> <branch>` it replaced, it does NOT DWIM a tracking
// branch from origin/<branch>. Any branch the clone never had locally (i.e.
// anything but the one `git clone` checked out) therefore landed with no
// upstream, and `git pull` in its worktree failed with "There is no tracking
// information for the current branch." The imported-branch path always set the
// upstream explicitly; the two PROTECTED paths did not.
//
// This exercises the protected path end to end: `develop` exists on origin and
// is in Crowbar's protected-branch fallback set, but the clone has never had it
// locally. After import, origin advances and the locked worktree must pull it.
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
	// `develop` is in Crowbar's protected-branch fallback set, and it must exist
	// on origin BEFORE the import so provisioning has to materialise it.
	runGit(t, seed, "checkout", "-b", "develop", "main")
	require.NoError(t, writeFile(seed, "d.txt", "d1\n"))
	runGit(t, seed, "add", "d.txt")
	runGit(t, seed, "commit", "-m", "d1")
	runGit(t, seed, "push", "-u", "origin", "develop")
	runGit(t, seed, "checkout", "main")

	// The clone Crowbar imports: it has `main` locally (git clone checked it out)
	// but has NEVER had a local `develop` — the case where `-B <sha>` leaves no
	// tracking info behind.
	repoPath := filepath.Join(root, "repo")
	runGit(t, root, "clone", origin, repoPath)
	runGit(t, repoPath, "config", "user.email", "t@t.dev")
	runGit(t, repoPath, "config", "user.name", "t")
	require.NotContains(t,
		runGitOut(t, repoPath, "branch", "--format=%(refname:short)"), "develop",
		"precondition: the clone must not already have a local develop")

	projectID, repoID := createProjectAndRepo(t, h, repoPath)
	// Read back rather than awaited on a stream: the repo-scoped chat feed has no
	// snapshot (agentChatDef), so a connection dialed after the async ImportRepo
	// job may already have missed develop's live frame. Quiesce is the barrier
	// that guarantees the job's provisioning has landed before the list is read.
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

	// origin moves ahead; the locked worktree must be able to pull it.
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

// BUG (import-hangs-on-unknown-branch): a branch that is not on the remote must
// be refused on the REQUEST, where the client can toast it. The import 202'd
// unconditionally and the async batch reported failure through
// runAsync(wsID: ""), which broadcastLastError drops on the floor — leaving the
// dialog's optimistic spinner row (cleared only by a workspace arriving on the
// stream) spinning forever with nothing surfaced.
func TestRegression_ImportRejectsBranchMissingFromRemote(t *testing.T) {
	h := newHarness(t)
	f := newImportFixture(t, h)

	resp := h.raw(http.MethodPost, f.repoBase()+"/chats/import-batch",
		map[string]any{"branches": []string{"feature/never-existed"}}, http.StatusBadRequest)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "feature/never-existed",
		"the refusal must name the branch so the client can surface it")
}
