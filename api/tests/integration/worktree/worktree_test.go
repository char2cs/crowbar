//go:build integration

package worktree_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// WorktreeSuite holds integration tests for the worktree usecase over the
// hierarchical, 202+WS workspace API (spec §3/§4/§5). The adopted main worktree
// (from ImportRepo) is locked (protected branch), so the suite's parent is an
// UNLOCKED child workspace forked from main; the tests' children fork from it.
type WorktreeSuite struct {
	kit.IntegrationSuite
	imported   kit.ImportedRepo
	parentID   string
	parentPath string
}

// SetupTest imports a repo and creates an unlocked parent workspace (a child of
// the locked adopted main) before each test.
func (s *WorktreeSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "worktree", "")
	s.parentID = s.Env.CreateWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/wt-base")
	s.parentPath = s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.parentID)
}

// TestWorktreeSuite runs the WorktreeSuite integration tests.
func TestWorktreeSuite(t *testing.T) {
	suite.Run(t, new(WorktreeSuite))
}

// wsBase returns the workspace-scoped route prefix for wsID.
func (s *WorktreeSuite) wsBase(wsID string) string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/workspaces/" + wsID
}

// reposBase returns the repo-scoped workspaces route prefix.
func (s *WorktreeSuite) reposBase() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID + "/workspaces"
}

// createChild creates a child workspace under the suite's parent and returns its
// id plus its on-disk worktree path.
func (s *WorktreeSuite) createChild(branch string) (string, string) {
	s.T().Helper()
	return s.createChildUnder(s.parentID, branch)
}

// createChildUnder creates a child workspace under parentID and returns its id
// plus its on-disk worktree path.
func (s *WorktreeSuite) createChildUnder(parentID, branch string) (string, string) {
	s.T().Helper()
	childID := s.Env.CreateChildWorkspace(
		s.T(),
		s.imported.ProjectID,
		s.imported.RepoID,
		branch,
		parentID,
	)
	return childID, s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)
}

// getWorkspace fetches a workspace by ID and returns the decoded DTO map.
func (s *WorktreeSuite) getWorkspace(id string) map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), s.wsBase(id))
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var ws map[string]any
	kit.DecodeEnvData(s.T(), resp, &ws)
	return ws
}

// listWorkspaces fetches the repo's workspaces and returns the decoded slice.
func (s *WorktreeSuite) listWorkspaces() []map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), s.reposBase())
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var list []map[string]any
	kit.DecodeEnvData(s.T(), resp, &list)
	return list
}

// mergeIntoParent triggers a child→parent merge (202) and blocks until the child
// WorkspaceDTO reaches the given terminal forkPointSha over WS (the merge
// updates the child's fork point to the new parent tip). Returns the parent's
// post-merge HEAD sha.
func (s *WorktreeSuite) mergeIntoParent(childID, strategy string) string {
	s.T().Helper()
	// Capture the child's pre-merge fork point so we can wait for the merge to
	// CHANGE it (a successful merge updates the child fork point to the new parent
	// tip; a fresh child's fork point is non-empty from creation, so "non-empty"
	// alone would race the connect snapshot).
	before := s.getWorkspace(childID)
	preMergeFork, _ := before["forkPointSha"].(string)

	watcher := s.Env.DialWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, childID)
	resp := s.Env.POST(s.T(), s.wsBase(childID)+"/merge-into-parent", map[string]any{
		"strategy": strategy,
	})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()
	merged := kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		fp, _ := m["forkPointSha"].(string)
		return fp != "" && fp != preMergeFork
	})
	parentTip, _ := merged["forkPointSha"].(string)
	return parentTip
}

// TestWorktree_createChildAddsWorktreeOnDisk verifies git worktree add runs.
func (s *WorktreeSuite) TestWorktree_createChildAddsWorktreeOnDisk() {
	t := s.T()

	childID, worktreePath := s.createChild("feature/create-test")

	s.Assert().True(kit.DirExists(t, worktreePath), "child worktree path must exist on disk")
	s.Assert().True(kit.BranchExists(t, s.imported.RepoPath, "feature/create-test"),
		"branch feature/create-test must exist after CreateChild")

	child := s.getWorkspace(childID)
	s.Assert().Equal(s.parentID, child["parentId"])

	kit.AssertWorkspaceConsistency(t, s.Env, childID)
}

// TestWorktree_mergeStrategyMerge verifies the fast-forward merge advances the parent.
func (s *WorktreeSuite) TestWorktree_mergeStrategyMerge() {
	t := s.T()

	childID, worktreePath := s.createChild("feature/merge-test")
	kit.CommitFile(t, worktreePath, "merge.txt", "child change\n", "child commit")

	parentTip := s.mergeIntoParent(childID, "merge")
	s.Assert().Equal(parentTip, kit.RevParse(t, s.parentPath, "HEAD"),
		"parent HEAD must match the merged tip")

	reloaded := s.getWorkspace(childID)
	s.Assert().Equal(parentTip, reloaded["forkPointSha"])
}

// TestWorktree_mergeStrategySquash verifies squash merge collapses child commits.
func (s *WorktreeSuite) TestWorktree_mergeStrategySquash() {
	t := s.T()

	parentTipBefore := kit.RevParse(t, s.parentPath, "HEAD")

	childID, worktreePath := s.createChild("feature/squash-test")
	kit.CommitFile(t, worktreePath, "a.txt", "a\n", "commit a")
	kit.CommitFile(t, worktreePath, "b.txt", "b\n", "commit b")

	s.mergeIntoParent(childID, "squash")

	parentTip := kit.RevParse(t, s.parentPath, "HEAD")
	s.Assert().NotEqual(parentTipBefore, parentTip, "squash must advance parent")

	parentMinus1 := kit.RevParse(t, s.parentPath, "HEAD~1")
	s.Assert().Equal(parentTipBefore, parentMinus1, "squash must produce exactly one new commit on parent")
}

// TestWorktree_mergeStrategyRebase verifies rebase + FF merge rewrites child SHA.
func (s *WorktreeSuite) TestWorktree_mergeStrategyRebase() {
	t := s.T()

	childID, worktreePath := s.createChild("feature/rebase-test")
	kit.CommitFile(t, worktreePath, "child.txt", "child\n", "child commit")
	childTipBefore := kit.RevParse(t, worktreePath, "HEAD")

	kit.CommitFile(t, s.parentPath, "parent.txt", "parent\n", "parent commit")

	parentTip := s.mergeIntoParent(childID, "rebase")

	childTipAfter := kit.RevParse(t, worktreePath, "HEAD")
	s.Assert().NotEqual(childTipBefore, childTipAfter, "rebase must rewrite child SHA")
	s.Assert().Equal(parentTip, childTipAfter, "FF-merge means parent == child tip")
}

// TestWorktree_reparent verifies leaf re-parenting replays child commits onto new parent.
func (s *WorktreeSuite) TestWorktree_reparent() {
	t := s.T()

	parentBID, parentBPath := s.createChild("feature/parent-b")
	kit.CommitFile(t, parentBPath, "pb.txt", "parent-b\n", "parent-b commit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	childID, childPath := s.createChild("feature/child")
	kit.CommitFile(t, childPath, "child.txt", "child\n", "child commit")

	watcher := s.Env.DialWorkspace(t, s.imported.ProjectID, s.imported.RepoID, childID)
	resp := s.Env.POST(t, s.wsBase(childID)+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["forkPointSha"] == parentBTip
	})

	s.Assert().True(kit.FileExists(t, childPath+"/child.txt"), "child's own commit must survive reparent")
	s.Assert().True(kit.FileExists(t, childPath+"/pb.txt"), "new parent's history must be in child branch")

	reloaded := s.getWorkspace(childID)
	s.Assert().Equal(parentBID, reloaded["parentId"])
	s.Assert().Equal(parentBTip, reloaded["forkPointSha"])
}

// TestWorktree_reparentWithChildrenRejected verifies the has_children guard. The
// guard runs in the background (00 §4: reparent is 202+async), so the rejection
// surfaces as a lastError on the workspace WS rather than a synchronous 409. The
// child's git history must be untouched.
func (s *WorktreeSuite) TestWorktree_reparentWithChildrenRejected() {
	t := s.T()

	parentBID, _ := s.createChild("feature/parent-b2")

	childID, childPath := s.createChild("feature/child-rejected")
	kit.CommitFile(t, childPath, "c.txt", "c\n", "child commit")
	childTipBefore := kit.RevParse(t, childPath, "HEAD")

	// Create a grandchild of `child` — Reparent must reject when child has children.
	s.createChildUnder(childID, "feature/grandchild-rejected")

	watcher := s.Env.DialWorkspace(t, s.imported.ProjectID, s.imported.RepoID, childID)
	resp := s.Env.POST(t, s.wsBase(childID)+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspaceLastError(t, watcher, childID, 5*time.Second)

	childTipAfterRejection := kit.RevParse(t, childPath, "HEAD")
	s.Assert().Equal(childTipBefore, childTipAfterRejection, "rejected reparent must not mutate git")

	// The parent must NOT have changed (rejected reparent).
	reloaded := s.getWorkspace(childID)
	s.Assert().Equal(s.parentID, reloaded["parentId"], "rejected reparent must keep the original parent")
}

// TestRegression_CreateWorkspace_RemoteBranchExists_Checkout proves the §3
// decision: when the requested branch already exists on the remote, CreateChild
// fetches origin/<branch> and checks it out (rather than forking a new branch
// from the parent). The remote-only file must appear in the child worktree.
func (s *WorktreeSuite) TestRegression_CreateWorkspace_RemoteBranchExists_Checkout() {
	t := s.T()

	repoPath, remoteBranch := kit.InitRepoWithRemoteBranch(t, "feature/remote-existing")
	imported := s.Env.ImportRepo(t, "remote-exists", repoPath)

	childID := s.Env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, remoteBranch)
	childPath := s.Env.WorktreePath(imported.ProjectID, imported.RepoID, childID)

	s.Assert().Equal(remoteBranch, kit.BranchName(t, childPath),
		"checked-out branch must match the remote branch")
	s.Assert().True(kit.FileExists(t, childPath+"/remote-only.txt"),
		"the remote branch's content must be checked out, not forked from parent")
}

// TestRegression_CreateWorkspace_RemoteBranchAbsent_CreateFromParent proves the
// other half of the §3 decision: a branch absent from the remote is created
// fresh from the parent branch (the remote-only content is NOT present).
func (s *WorktreeSuite) TestRegression_CreateWorkspace_RemoteBranchAbsent_CreateFromParent() {
	t := s.T()

	repoPath, _ := kit.InitRepoWithRemoteBranch(t, "feature/remote-existing")
	imported := s.Env.ImportRepo(t, "remote-absent", repoPath)

	childID := s.Env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/brand-new")
	childPath := s.Env.WorktreePath(imported.ProjectID, imported.RepoID, childID)

	s.Assert().Equal("feature/brand-new", kit.BranchName(t, childPath))
	s.Assert().False(kit.FileExists(t, childPath+"/remote-only.txt"),
		"a branch absent from the remote must fork from parent, not carry remote-only content")
	s.Assert().True(kit.FileExists(t, childPath+"/base.txt"),
		"a fresh branch forked from parent must carry the parent's content")
}

// TestRegression_MergeEligibility_SiblingState proves the §10 sibling-scan rule:
// a child's canMergeLocally flips with its parent's status. An idle parent yields
// {true, parentBranch}; a locked parent yields {false, ""}.
func (s *WorktreeSuite) TestRegression_MergeEligibility_SiblingState() {
	t := s.T()

	parentID, _ := s.createChild("feature/elig-parent")
	watcher := s.Env.DialWorkspaces(t, s.imported.ProjectID, s.imported.RepoID)
	childID := s.Env.CreateChildWorkspace(t, s.imported.ProjectID, s.imported.RepoID, "feature/elig-child", parentID)

	// Idle parent → child can merge locally onto the parent branch.
	kit.WaitForWorkspace(t, watcher, childID, 5*time.Second, func(m map[string]any) bool {
		can, _ := m["canMergeLocally"].(bool)
		return can && m["parentBranch"] == "feature/elig-parent"
	})

	// Lock the parent → the child's eligibility flips to false.
	parentWatcher := s.Env.DialWorkspace(t, s.imported.ProjectID, s.imported.RepoID, parentID)
	s.Env.PushProviderState(t, parentID, kit.ProviderState{Protected: true})
	kit.WaitForWorkspaceState(t, parentWatcher, parentID, "locked", 5*time.Second)

	child := s.getWorkspace(childID)
	can, _ := child["canMergeLocally"].(bool)
	s.Assert().False(can, "a locked parent must make the child ineligible to merge locally")
}

// TestWorktree_deleteCascadeSkipsLockedChild verifies cascade delete preserves a
// locked child. The child is locked via the provider seam (Protected:true →
// Status=locked) since locking is status-based now (spec §5), not a create-body
// flag. Delete is 202 + a deleted-status tombstone on the WS.
func (s *WorktreeSuite) TestWorktree_deleteCascadeSkipsLockedChild() {
	t := s.T()

	rootID, _ := s.createChild("feature/root-cascade")
	lockedID, _ := s.createChildUnder(rootID, "feature/locked-branch")

	// Lock the child via a protected provider state.
	lockWatcher := s.Env.DialWorkspace(t, s.imported.ProjectID, s.imported.RepoID, lockedID)
	s.Env.PushProviderState(t, lockedID, kit.ProviderState{Protected: true})
	kit.WaitForWorkspaceState(t, lockWatcher, lockedID, "locked", 5*time.Second)

	rootWatcher := s.Env.DialWorkspace(t, s.imported.ProjectID, s.imported.RepoID, rootID)
	resp := s.Env.DELETE(t, s.wsBase(rootID))
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspaceState(t, rootWatcher, rootID, "deleted", 5*time.Second)

	all := s.listWorkspaces()
	ids := map[string]bool{}
	for _, ws := range all {
		ids[ws["id"].(string)] = true
	}
	s.Assert().True(ids[lockedID], "locked child must survive cascade delete")
	s.Assert().False(ids[rootID], "unlocked root must be removed")
}
