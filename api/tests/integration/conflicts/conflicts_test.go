//go:build integration

package conflicts_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test harness entry point.
func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

// ConflictsSuite tests conflict detection, hunk parsing, and merge-abort flows
// against a real multi-worktree git repository.
type ConflictsSuite struct {
	kit.IntegrationSuite
	repoPath   string
	baseBranch string
	parentID   string
}

// SetupTest initialises a fresh environment, repo, and parent workspace for each test case.
func (s *ConflictsSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()

	s.repoPath = kit.InitRepo(s.T())
	kit.GitRun(s.T(), s.repoPath, "branch", "-m", "main", "feature/conflicts-base")
	s.baseBranch = kit.BranchName(s.T(), s.repoPath)

	// Register the repo via HTTP.
	resp := s.Env.POST(s.T(), "/v0/repos", map[string]string{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(s.T(), resp, 201)
	resp.Body.Close()

	// Register the parent workspace via HTTP.
	resp = s.Env.POST(s.T(), "/v0/workspaces", map[string]string{
		"repoId": "r1",
		"branch": s.baseBranch,
	})
	kit.RequireStatus(s.T(), resp, 201)
	s.parentID = kit.MutationID(s.T(), resp)
}

// TestConflictsSuite runs the conflict resolution integration suite.
func TestConflictsSuite(t *testing.T) {
	suite.Run(
		t,
		new(ConflictsSuite),
	)
}

// conflictSetup commits a base file, creates a child workspace, then commits
// diverging edits on both child and parent to produce a merge conflict.
// Returns the child workspace ID and its worktree path.
func (s *ConflictsSuite) conflictSetup() (childID string, childWorktreePath string) {
	s.T().Helper()

	// Commit the base version of shared.txt on the parent branch.
	kit.CommitFile(
		s.T(),
		s.repoPath,
		"shared.txt",
		"base line\n",
		"base",
	)

	// Create a child workspace via HTTP.
	resp := s.Env.POST(s.T(), "/v0/workspaces", map[string]string{
		"repoId":   "r1",
		"branch":   "feature/conflict",
		"parentId": s.parentID,
	})
	kit.RequireStatus(s.T(), resp, 201)
	childID = kit.MutationID(s.T(), resp)

	// GET the child workspace to retrieve the worktree path.
	getResp := s.Env.GET(s.T(), "/v0/workspaces/"+childID)
	kit.RequireStatus(s.T(), getResp, http.StatusOK)
	var childWs map[string]any
	kit.DecodeEnvData(s.T(), getResp, &childWs)
	childWorktreePath = childWs["worktreePath"].(string)

	// Commit a diverging edit on the child branch.
	kit.CommitFile(
		s.T(),
		childWorktreePath,
		"shared.txt",
		"child version\n",
		"child edit",
	)

	// Commit a diverging edit on the parent branch.
	kit.CommitFile(
		s.T(),
		s.repoPath,
		"shared.txt",
		"parent version\n",
		"parent edit",
	)

	return childID, childWorktreePath
}

// TestConflicts_mergeDetectsConflict verifies that a conflicting merge sets PendingMerge.
func (s *ConflictsSuite) TestConflicts_mergeDetectsConflict() {
	childID, _ := s.conflictSetup()

	// Merge child into parent via HTTP.
	resp := s.Env.POST(s.T(), fmt.Sprintf("/v0/workspaces/%s/merge-into-parent", childID), map[string]string{
		"strategy": "merge",
	})
	kit.RequireStatus(s.T(), resp, 200)

	var mergeResult map[string]any
	kit.DecodeEnvData(s.T(), resp, &mergeResult)

	s.Assert().Equal(true, mergeResult["conflictsPending"], "merge must report conflicts pending")
	s.Assert().Empty(mergeResult["parentTipSha"], "parent tip SHA must be empty on conflict")

	// Reload the child workspace and verify PendingMerge is set.
	getResp := s.Env.GET(s.T(), fmt.Sprintf("/v0/workspaces/%s", childID))
	kit.RequireStatus(s.T(), getResp, 200)

	var reloaded map[string]any
	kit.DecodeEnvData(s.T(), getResp, &reloaded)

	s.Require().NotNil(reloaded["pendingMerge"], "child workspace must have pendingMerge set after conflict")

	pendingMerge, ok := reloaded["pendingMerge"].(map[string]any)
	s.Require().True(ok, "pendingMerge must be a JSON object")
	s.Assert().Equal("merge", pendingMerge["strategy"])
	s.Assert().Equal(s.parentID, pendingMerge["targetParentId"])
}

// TestConflicts_conflictedFilesListsFile verifies the conflicted-files endpoint
// returns the shared file that triggered the merge conflict.
func (s *ConflictsSuite) TestConflicts_conflictedFilesListsFile() {
	childID, _ := s.conflictSetup()

	// Trigger a conflicting merge.
	resp := s.Env.POST(s.T(), fmt.Sprintf("/v0/workspaces/%s/merge-into-parent", childID), map[string]string{
		"strategy": "merge",
	})
	kit.RequireStatus(s.T(), resp, 200)

	var mergeResult map[string]any
	kit.DecodeEnvData(s.T(), resp, &mergeResult)
	s.Require().Equal(true, mergeResult["conflictsPending"])

	// GET the conflicted files on the parent workspace.
	conflictsResp := s.Env.GET(s.T(), fmt.Sprintf("/v0/workspaces/%s/git/conflicts", s.parentID))
	kit.RequireStatus(s.T(), conflictsResp, 200)

	var files []string
	kit.DecodeJSON(s.T(), conflictsResp, &files)

	s.Require().NotEmpty(files)
	s.Assert().Contains(files, "shared.txt")
}

// TestConflicts_conflictHunksParsesThreeWayView verifies conflict hunk parsing
// returns ours/theirs sections for the conflicted file.
func (s *ConflictsSuite) TestConflicts_conflictHunksParsesThreeWayView() {
	childID, _ := s.conflictSetup()

	// Trigger a conflicting merge.
	resp := s.Env.POST(s.T(), fmt.Sprintf("/v0/workspaces/%s/merge-into-parent", childID), map[string]string{
		"strategy": "merge",
	})
	kit.RequireStatus(s.T(), resp, 200)

	var mergeResult map[string]any
	kit.DecodeEnvData(s.T(), resp, &mergeResult)
	s.Require().Equal(true, mergeResult["conflictsPending"])

	// GET conflict hunks for shared.txt on the parent workspace.
	hunksResp := s.Env.GET(s.T(), fmt.Sprintf("/v0/workspaces/%s/git/conflict-hunks?path=shared.txt", s.parentID))
	kit.RequireStatus(s.T(), hunksResp, 200)

	var hunks []map[string]any
	kit.DecodeJSON(s.T(), hunksResp, &hunks)

	s.Require().NotEmpty(hunks)
	s.Assert().NotEmpty(hunks[0]["ours"])
	s.Assert().NotEmpty(hunks[0]["theirs"])
}

// TestConflicts_operationAbortRestoresCleanParent verifies git merge --abort
// restores the parent tree to pre-merge state.
func (s *ConflictsSuite) TestConflicts_operationAbortRestoresCleanParent() {
	childID, _ := s.conflictSetup()

	// Trigger a conflicting merge.
	resp := s.Env.POST(s.T(), fmt.Sprintf("/v0/workspaces/%s/merge-into-parent", childID), map[string]string{
		"strategy": "merge",
	})
	kit.RequireStatus(s.T(), resp, 200)

	var mergeResult map[string]any
	kit.DecodeEnvData(s.T(), resp, &mergeResult)
	s.Require().Equal(true, mergeResult["conflictsPending"])

	// Abort the merge on the parent workspace.
	abortResp := s.Env.POST(s.T(), fmt.Sprintf("/v0/workspaces/%s/git/operation/abort", s.parentID), nil)
	kit.RequireStatus(s.T(), abortResp, 200)

	// Verify the parent working tree is clean after abort.
	statusResp := s.Env.GET(s.T(), fmt.Sprintf("/v0/workspaces/%s/git/status", s.parentID))
	kit.RequireStatus(s.T(), statusResp, 200)

	var status map[string]any
	kit.DecodeJSON(s.T(), statusResp, &status)

	files, _ := status["files"].([]any)
	s.Assert().Empty(files, "parent must be clean after merge abort")

	// Reload the child workspace and verify PendingMerge is still set
	// (abort must not auto-clear the pending merge marker on the child).
	childResp := s.Env.GET(s.T(), fmt.Sprintf("/v0/workspaces/%s", childID))
	kit.RequireStatus(s.T(), childResp, 200)

	var childWs map[string]any
	kit.DecodeEnvData(s.T(), childResp, &childWs)

	s.Require().NotNil(
		childWs["pendingMerge"],
		"abort must not auto-clear pending merge on the child workspace",
	)
}
