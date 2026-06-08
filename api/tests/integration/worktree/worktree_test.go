//go:build integration

package worktree_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// WorktreeSuite holds integration tests for the worktree usecase.
type WorktreeSuite struct {
	kit.IntegrationSuite
	repoPath   string
	baseBranch string
	parentID   string
}

// SetupTest initialises a fresh Env, git repository, and parent workspace before each test.
func (s *WorktreeSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()

	s.repoPath = kit.InitRepo(s.T())
	kit.GitRun(s.T(), s.repoPath, "branch", "-m", "main", "feature/wt-base")
	s.baseBranch = kit.BranchName(s.T(), s.repoPath)

	// Register the repo.
	resp := s.Env.POST(s.T(), "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	resp.Body.Close()

	// Register the parent workspace.
	resp = s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
		"repoId": "r1",
		"branch": s.baseBranch,
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	s.parentID = kit.MutationID(s.T(), resp)
}

// TestWorktreeSuite runs the WorktreeSuite integration tests.
func TestWorktreeSuite(t *testing.T) {
	suite.Run(t, new(WorktreeSuite))
}

// createChild posts POST /v0/workspaces with parentId and returns the decoded workspace map.
func (s *WorktreeSuite) createChild(branch string) map[string]any {
	s.T().Helper()
	resp := s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
		"repoId":   "r1",
		"branch":   branch,
		"parentId": s.parentID,
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	childID := kit.MutationID(s.T(), resp)
	return s.getWorkspace(childID)
}

// createChildUnder posts POST /v0/workspaces with a custom parentId and returns the decoded workspace map.
func (s *WorktreeSuite) createChildUnder(parentID, parentBranch, branch string) map[string]any {
	s.T().Helper()
	resp := s.Env.POST(s.T(), "/v0/workspaces", map[string]any{
		"repoId":   "r1",
		"branch":   branch,
		"parentId": parentID,
	})
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	childID := kit.MutationID(s.T(), resp)
	return s.getWorkspace(childID)
}

// getWorkspace fetches a workspace by ID and returns the decoded map.
func (s *WorktreeSuite) getWorkspace(id string) map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), "/v0/workspaces/"+id)
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var ws map[string]any
	kit.DecodeEnvData(s.T(), resp, &ws)
	return ws
}

// listWorkspaces fetches all workspaces and returns the decoded slice.
func (s *WorktreeSuite) listWorkspaces() []map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), "/v0/workspaces")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var list []map[string]any
	kit.DecodeEnvData(s.T(), resp, &list)
	return list
}

// TestWorktree_createChildAddsWorktreeOnDisk verifies git worktree add runs.
func (s *WorktreeSuite) TestWorktree_createChildAddsWorktreeOnDisk() {
	t := s.T()

	child := s.createChild("feature/create-test")
	worktreePath := child["worktreePath"].(string)

	worktreeExists := kit.DirExists(t, worktreePath)
	s.Assert().True(worktreeExists, "child worktree path must exist on disk")

	branchExists := kit.BranchExists(t, s.repoPath, "feature/create-test")
	s.Assert().True(branchExists, "branch feature/create-test must exist in the repo after CreateChild")

	s.Assert().Equal(s.parentID, child["parentId"])

	childID := child["id"].(string)
	kit.AssertWorkspaceConsistency(t, s.Env, childID)
}

// TestWorktree_mergeStrategyMerge verifies the fast-forward merge advances the parent.
func (s *WorktreeSuite) TestWorktree_mergeStrategyMerge() {
	t := s.T()

	child := s.createChild("feature/merge-test")
	childID := child["id"].(string)
	worktreePath := child["worktreePath"].(string)

	kit.CommitFile(t, worktreePath, "merge.txt", "child change\n", "child commit")

	resp := s.Env.POST(t, "/v0/workspaces/"+childID+"/merge-into-parent", map[string]any{
		"strategy": "merge",
	})
	kit.RequireStatus(t, resp, http.StatusOK)

	var res map[string]any
	kit.DecodeEnvData(t, resp, &res)

	s.Assert().Equal(false, res["conflictsPending"])
	s.Assert().NotEmpty(res["parentTipSha"])

	parentTip := kit.RevParse(t, s.repoPath, "HEAD")
	s.Assert().Equal(res["parentTipSha"], parentTip, "parent HEAD must match usecase-returned tip")

	reloaded := s.getWorkspace(childID)
	s.Assert().Equal(res["parentTipSha"], reloaded["forkPointSha"])
}

// TestWorktree_mergeStrategySquash verifies squash merge collapses child commits.
func (s *WorktreeSuite) TestWorktree_mergeStrategySquash() {
	t := s.T()

	parentTipBefore := kit.RevParse(t, s.repoPath, "HEAD")

	child := s.createChild("feature/squash-test")
	childID := child["id"].(string)
	worktreePath := child["worktreePath"].(string)

	kit.CommitFile(t, worktreePath, "a.txt", "a\n", "commit a")
	kit.CommitFile(t, worktreePath, "b.txt", "b\n", "commit b")

	resp := s.Env.POST(t, "/v0/workspaces/"+childID+"/merge-into-parent", map[string]any{
		"strategy": "squash",
	})
	kit.RequireStatus(t, resp, http.StatusOK)

	var res map[string]any
	kit.DecodeEnvData(t, resp, &res)

	s.Assert().Equal(false, res["conflictsPending"])

	parentTip := kit.RevParse(t, s.repoPath, "HEAD")
	s.Assert().NotEqual(parentTipBefore, parentTip, "squash must advance parent")

	parentMinus1 := kit.RevParse(t, s.repoPath, "HEAD~1")
	s.Assert().Equal(parentTipBefore, parentMinus1, "squash must produce exactly one new commit on parent")
}

// TestWorktree_mergeStrategyRebase verifies rebase + FF merge rewrites child SHA.
func (s *WorktreeSuite) TestWorktree_mergeStrategyRebase() {
	t := s.T()

	child := s.createChild("feature/rebase-test")
	childID := child["id"].(string)
	worktreePath := child["worktreePath"].(string)

	kit.CommitFile(t, worktreePath, "child.txt", "child\n", "child commit")
	childTipBefore := kit.RevParse(t, worktreePath, "HEAD")

	kit.CommitFile(t, s.repoPath, "parent.txt", "parent\n", "parent commit")

	resp := s.Env.POST(t, "/v0/workspaces/"+childID+"/merge-into-parent", map[string]any{
		"strategy": "rebase",
	})
	kit.RequireStatus(t, resp, http.StatusOK)

	var res map[string]any
	kit.DecodeEnvData(t, resp, &res)

	s.Assert().Equal(false, res["conflictsPending"])

	childTipAfter := kit.RevParse(t, worktreePath, "HEAD")
	s.Assert().NotEqual(childTipBefore, childTipAfter, "rebase must rewrite child SHA")
	s.Assert().Equal(res["parentTipSha"], childTipAfter, "FF-merge means parent == child tip")
}

// TestWorktree_reparent verifies leaf re-parenting replays child commits onto new parent.
func (s *WorktreeSuite) TestWorktree_reparent() {
	t := s.T()

	parentB := s.createChild("feature/parent-b")
	parentBID := parentB["id"].(string)
	parentBPath := parentB["worktreePath"].(string)

	kit.CommitFile(t, parentBPath, "pb.txt", "parent-b\n", "parent-b commit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	child := s.createChild("feature/child")
	childID := child["id"].(string)
	childPath := child["worktreePath"].(string)

	kit.CommitFile(t, childPath, "child.txt", "child\n", "child commit")

	resp := s.Env.POST(t, "/v0/workspaces/"+childID+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	kit.RequireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	childTxtExists := kit.FileExists(t, childPath+"/child.txt")
	s.Assert().True(childTxtExists, "child's own commit must survive reparent")

	pbTxtExists := kit.FileExists(t, childPath+"/pb.txt")
	s.Assert().True(pbTxtExists, "new parent's history must be in child branch")

	reloaded := s.getWorkspace(childID)
	s.Assert().Equal(parentBID, reloaded["parentId"])
	s.Assert().Equal(parentBTip, reloaded["forkPointSha"])
}

// TestWorktree_reparentWithChildrenRejected verifies has_children guard.
func (s *WorktreeSuite) TestWorktree_reparentWithChildrenRejected() {
	t := s.T()

	parentB := s.createChild("feature/parent-b2")
	parentBID := parentB["id"].(string)

	child := s.createChild("feature/child-rejected")
	childID := child["id"].(string)
	childPath := child["worktreePath"].(string)

	kit.CommitFile(t, childPath, "c.txt", "c\n", "child commit")
	childTipBefore := kit.RevParse(t, childPath, "HEAD")

	// Create a grandchild of `child` — Reparent must reject when child has children.
	s.createChildUnder(childID, "feature/child-rejected", "feature/grandchild-rejected")

	resp := s.Env.POST(t, "/v0/workspaces/"+childID+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	s.Assert().Equal(http.StatusConflict, resp.StatusCode)
	resp.Body.Close()

	childTipAfterRejection := kit.RevParse(t, childPath, "HEAD")
	s.Assert().Equal(childTipBefore, childTipAfterRejection, "rejected reparent must not mutate git")
}

// TestWorktree_deleteCascadeSkipsLockedChild verifies locked children are preserved.
func (s *WorktreeSuite) TestWorktree_deleteCascadeSkipsLockedChild() {
	t := s.T()

	root := s.createChild("feature/root-cascade")
	rootID := root["id"].(string)

	// Create a locked child workspace — cascade delete must skip it.
	resp := s.Env.POST(t, "/v0/workspaces", map[string]any{
		"repoId":   "r1",
		"branch":   "feature/locked-branch",
		"parentId": rootID,
		"locked":   true,
	})
	kit.RequireStatus(t, resp, http.StatusCreated)
	lockedID := kit.MutationID(t, resp)

	resp = s.Env.DELETE(t, "/v0/workspaces/"+rootID)
	kit.RequireStatus(t, resp, http.StatusOK)
	kit.MutationID(t, resp) // consume body

	all := s.listWorkspaces()

	ids := map[string]bool{}
	for _, ws := range all {
		ids[ws["id"].(string)] = true
	}

	s.Assert().True(ids[lockedID], "locked child must survive cascade delete")
	s.Assert().False(ids[rootID], "unlocked root must be removed")
}
