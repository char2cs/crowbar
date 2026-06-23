//go:build integration

package conflicts_test

import (
	"encoding/json"
	"io"
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

// mergeConflict triggers a conflicting child→parent merge (202) and blocks until
// the async merge has actually run and left conflict markers in the parent
// worktree.
//
// It cannot key on the child reaching Status=pr-conflicts: that status is now
// also produced up front by the merge-tree prediction overlay (a child with
// diverging edits reads pr-conflicts before any merge is attempted), so it flips
// before the operation runs. The real post-condition is unmerged paths in the
// parent — poll for them directly.
func (s *ConflictsSuite) mergeConflict(childID string) {
	s.T().Helper()
	resp := s.Env.POST(s.T(), s.wsBase(childID)+"/merge-into-parent", map[string]string{
		"strategy": "merge",
	})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()
	s.waitForParentConflicts(5 * time.Second)
}

// waitForParentConflicts polls the parent's /git/conflicts endpoint until it
// lists at least one unmerged file, failing the test if the timeout elapses.
// Decoding is inlined (not kit.DecodeEnvData) so an intermediate empty/odd
// response just retries instead of failing the run.
func (s *ConflictsSuite) waitForParentConflicts(timeout time.Duration) {
	s.T().Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp := s.Env.GET(s.T(), s.wsBase(s.parentID)+"/git/conflicts")
		if resp.StatusCode == http.StatusOK {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var env struct {
				Data []string `json:"data"`
			}
			if json.Unmarshal(raw, &env) == nil && len(env.Data) > 0 {
				return
			}
		} else {
			resp.Body.Close()
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.FailNow("parent worktree never reported conflict markers within timeout")
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
// TestConflicts_mergeConflictsPredictedBeforeMerge verifies the workspace DTO
// reports mergeConflicts:true when folding the child into its parent WOULD
// conflict — computed up front, before any merge is attempted, so the UI can
// block the merge. canMergeLocally stays structurally true.
func (s *ConflictsSuite) TestConflicts_mergeConflictsPredictedBeforeMerge() {
	childID := s.conflictSetup() // diverging edits to shared.txt on child + parent; no merge

	getResp := s.Env.GET(s.T(), s.wsBase(childID))
	kit.RequireStatus(s.T(), getResp, http.StatusOK)
	var ws map[string]any
	kit.DecodeEnvData(s.T(), getResp, &ws)

	s.Assert().Equal(true, ws["mergeConflicts"],
		"a child that would conflict must report mergeConflicts:true before any merge")
	s.Assert().Equal(true, ws["canMergeLocally"],
		"canMergeLocally stays structurally true")
}

// TestConflicts_mergeConflictsDeliveredOnBroadcast verifies the LIVE workspace
// broadcast (not only the snapshot/REST read) carries the predicted
// mergeConflicts flag — the broadcast and snapshot paths share one resolver. A
// benign mutation (set merge strategy) triggers a broadcast; the predicate waits
// for that post-mutation frame, so it exercises the broadcast path specifically.
func (s *ConflictsSuite) TestConflicts_mergeConflictsDeliveredOnBroadcast() {
	childID := s.conflictSetup() // conflict between child + parent, no merge

	watcher := s.Env.DialWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, childID)
	resp := s.Env.PATCH(s.T(), s.wsBase(childID)+"/review", map[string]any{"mergeStrategy": "squash"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	got := kit.WaitForWorkspace(s.T(), watcher, childID, 5*time.Second, func(m map[string]any) bool {
		return m["mergeStrategy"] == "squash"
	})
	s.Assert().Equal(true, got["mergeConflicts"],
		"the live broadcast must carry the predicted merge conflict, not only the snapshot read")
}

// TestConflicts_mergeDeleteSourceKeepsConflictedChild verifies that a conflicting
// merge requested with deleteSource:true does NOT delete the child — the conflict
// must be resolved first, so deleting it would lose the user's work. The child
// stays at pr-conflicts.
func (s *ConflictsSuite) TestConflicts_mergeDeleteSourceKeepsConflictedChild() {
	childID := s.conflictSetup()

	watcher := s.Env.DialWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, childID)
	resp := s.Env.POST(s.T(), s.wsBase(childID)+"/merge-into-parent", map[string]any{
		"strategy":     "merge",
		"deleteSource": true,
	})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspaceState(s.T(), watcher, childID, "pr-conflicts", 5*time.Second)

	// Despite deleteSource:true, the conflicted child must survive.
	getResp := s.Env.GET(s.T(), s.wsBase(childID))
	kit.RequireStatus(s.T(), getResp, http.StatusOK)
}

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
