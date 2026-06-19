//go:build integration

package conflicts_test

import (
	"net/http"
	"testing"
	"time"

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
// against a real multi-worktree git repository. A local merge conflict now
// transitions the child workspace to Status=pr-conflicts (00 §6.1) — there is no
// PendingMerge struct anymore (spec §5) — and merge-into-parent is 202+WS.
type ConflictsSuite struct {
	kit.IntegrationSuite
	imported   kit.ImportedRepo
	parentID   string
	parentPath string
}

// SetupTest imports a repo and creates an UNLOCKED parent workspace (a child of
// the locked adopted main) for each test case. Merging into a locked parent is
// rejected by the guard, so the parent must be an unlocked feature branch.
func (s *ConflictsSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "conflicts", "")
	s.parentID = s.Env.CreateWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/conflicts-base")
	s.parentPath = s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.parentID)
}

// wsBase returns the workspace-scoped route prefix for the given workspace id.
func (s *ConflictsSuite) wsBase(wsID string) string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/workspaces/" + wsID
}

// repoBase returns the repo-scoped route prefix.
func (s *ConflictsSuite) repoBase() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID
}

// TestConflictsSuite runs the conflict resolution integration suite.
func TestConflictsSuite(t *testing.T) {
	suite.Run(
		t,
		new(ConflictsSuite),
	)
}

// conflictSetup commits a base file on the parent (adopted-main) worktree,
// creates a child workspace, then commits diverging edits on both child and
// parent to produce a merge conflict. Returns the child workspace ID.
func (s *ConflictsSuite) conflictSetup() (childID string) {
	s.T().Helper()

	// Commit the base version of shared.txt on the (unlocked) parent worktree.
	kit.CommitFile(
		s.T(),
		s.parentPath,
		"shared.txt",
		"base line\n",
		"base",
	)

	childID = s.Env.CreateChildWorkspace(
		s.T(),
		s.imported.ProjectID,
		s.imported.RepoID,
		"feature/conflict",
		s.parentID,
	)
	childWorktreePath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)

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
		s.parentPath,
		"shared.txt",
		"parent version\n",
		"parent edit",
	)
	return childID
}

// mergeConflict triggers a conflicting child→parent merge (202) and blocks on
// the child WorkspaceDTO reaching Status=pr-conflicts over WS.
func (s *ConflictsSuite) mergeConflict(childID string) {
	s.T().Helper()
	watcher := s.Env.DialWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, childID)
	resp := s.Env.POST(s.T(), s.wsBase(childID)+"/merge-into-parent", map[string]string{
		"strategy": "merge",
	})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspaceState(s.T(), watcher, childID, "pr-conflicts", 5*time.Second)
}

// TestConflicts_mergeDetectsConflict verifies a conflicting merge transitions
// the child to Status=pr-conflicts (broadcast over WS).
func (s *ConflictsSuite) TestConflicts_mergeDetectsConflict() {
	childID := s.conflictSetup()
	s.mergeConflict(childID)

	getResp := s.Env.GET(s.T(), s.wsBase(childID))
	kit.RequireStatus(s.T(), getResp, http.StatusOK)
	var reloaded map[string]any
	kit.DecodeEnvData(s.T(), getResp, &reloaded)
	s.Assert().Equal("pr-conflicts", reloaded["status"],
		"child workspace must be pr-conflicts after a conflicting merge")
}

// TestConflicts_conflictedFilesListsFile verifies the conflicted-files endpoint
// returns the shared file that triggered the merge conflict.
func (s *ConflictsSuite) TestConflicts_conflictedFilesListsFile() {
	childID := s.conflictSetup()
	s.mergeConflict(childID)

	conflictsResp := s.Env.GET(s.T(), s.wsBase(s.parentID)+"/git/conflicts")
	kit.RequireStatus(s.T(), conflictsResp, http.StatusOK)

	var files []string
	kit.DecodeEnvData(s.T(), conflictsResp, &files)
	s.Require().NotEmpty(files)
	s.Assert().Contains(files, "shared.txt")
}

// TestConflicts_conflictHunksParsesThreeWayView verifies conflict hunk parsing
// returns ours/theirs sections for the conflicted file.
func (s *ConflictsSuite) TestConflicts_conflictHunksParsesThreeWayView() {
	childID := s.conflictSetup()
	s.mergeConflict(childID)

	hunksResp := s.Env.GET(s.T(), s.wsBase(s.parentID)+"/git/conflict-hunks?path=shared.txt")
	kit.RequireStatus(s.T(), hunksResp, http.StatusOK)

	var hunks []map[string]any
	kit.DecodeEnvData(s.T(), hunksResp, &hunks)
	s.Require().NotEmpty(hunks)
	s.Assert().NotEmpty(hunks[0]["ours"])
	s.Assert().NotEmpty(hunks[0]["theirs"])
}

// TestConflicts_operationAbortRestoresCleanParent verifies git merge --abort
// restores the parent tree to pre-merge state and does not auto-clear the
// child's pr-conflicts status.
func (s *ConflictsSuite) TestConflicts_operationAbortRestoresCleanParent() {
	childID := s.conflictSetup()
	s.mergeConflict(childID)

	abortResp := s.Env.POST(s.T(), s.wsBase(s.parentID)+"/git/operation/abort", nil)
	kit.RequireStatus(s.T(), abortResp, http.StatusOK)
	abortResp.Body.Close()

	statusResp := s.Env.GET(s.T(), s.wsBase(s.parentID)+"/git/status")
	kit.RequireStatus(s.T(), statusResp, http.StatusOK)
	var status map[string]any
	kit.DecodeEnvData(s.T(), statusResp, &status)
	files, _ := status["files"].([]any)
	s.Assert().Empty(files, "parent must be clean after merge abort")

	childResp := s.Env.GET(s.T(), s.wsBase(childID))
	kit.RequireStatus(s.T(), childResp, http.StatusOK)
	var childWs map[string]any
	kit.DecodeEnvData(s.T(), childResp, &childWs)
	s.Assert().Equal("pr-conflicts", childWs["status"],
		"abort must not auto-clear the child's pr-conflicts status")
}
