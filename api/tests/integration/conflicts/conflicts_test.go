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

// ConflictsSuite tests conflict detection, hunk parsing, and the merge
// try-then-warn model against a real multi-worktree git repository. A
// conflicting merge-into-parent now ABORTS the in-progress op (never leaving a
// stuck worktree, H6/H7) and surfaces the conflict ONLY as the child's
// Status=pr-conflicts (00 §6.1) — both worktrees stay clean; the user resolves
// via "Rebase onto parent" (a resolvable rebase kept in the child's OWN
// worktree) and re-runs the merge. The general /git/conflicts + /git/conflict-
// hunks endpoints are therefore exercised against a kept-rebase on the child,
// since merge-into-parent no longer leaves markers anywhere. merge-into-parent
// is 202+WS.
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
// the async merge has run and settled into the try-then-warn end state: the
// child reaches Status=pr-conflicts AND BOTH worktrees are clean (the merge was
// aborted, never left stuck). It dials the child WS and waits for the
// post-merge pr-conflicts frame, then asserts cleanliness directly.
//
// It cannot simply key on the child reaching Status=pr-conflicts via REST: that
// status is also produced up front by the merge-tree prediction overlay (a child
// with diverging edits reads pr-conflicts before any merge is attempted). The
// reliable post-condition is the worktrees having been aborted clean — so the
// helper waits on the WS frame after the merge and then verifies both worktrees.
func (s *ConflictsSuite) mergeConflict(childID string) {
	s.T().Helper()
	childPath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)

	watcher := s.Env.DialWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, childID)
	resp := s.Env.POST(s.T(), s.wsBase(childID)+"/merge-into-parent", map[string]string{
		"strategy": "merge",
	})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()

	// Block on the MERGE OP finishing, not on the child reaching pr-conflicts.
	//
	// The status is the wrong signal and always was: pr-conflicts is ALSO produced up
	// front by the merge-tree prediction overlay (a child with diverging edits reads
	// pr-conflicts before a merge is even attempted), so waiting for it can return
	// while the real merge is still mid-conflict, with the worktree not yet aborted.
	// The old code covered that hole by polling the worktrees for cleanliness for two
	// seconds — a guess that the abort would land inside the window.
	//
	// The daemon's working overlay says exactly what is needed: the detached merge op
	// is bracketed BeginWork → EndWork, and EndWork fires only after the merge
	// function has returned, abort and all. The falling edge IS "the merge is over".
	final := kit.WaitForWorkComplete(s.T(), watcher, childID, 30*time.Second)
	s.Require().Equal("pr-conflicts", final["status"],
		"a conflicting merge must leave the child in pr-conflicts")
	s.Env.Quiesce()

	// H6/H7 regression guard: a conflicting merge must leave NEITHER worktree stuck.
	// The merge runs in the parent, so the parent's in-progress merge is aborted
	// clean; the child was never touched. Both are now plain assertions on a settled
	// filesystem.
	s.requireClean(childPath, "child")
	s.requireClean(s.parentPath, "parent")
}

// requireClean asserts a worktree has no `git status --porcelain` output AND that
// HEAD is attached to a branch (not a detached mid-rebase HEAD), proving no
// conflict markers and no in-progress operation remain. A conflicting squash shows
// "UU <file>" in porcelain and a mid-rebase detaches HEAD, so an empty porcelain
// on an attached branch reliably means clean+not-stuck.
//
// It is a plain assertion, not a poll: its caller has already drained the async
// merge that would otherwise still be writing (see mergeConflict).
func (s *ConflictsSuite) requireClean(worktreePath, label string) {
	s.T().Helper()
	porcelain := kit.TrimNewline(kit.GitRun(s.T(), worktreePath, "status", "--porcelain"))
	head := kit.TrimNewline(kit.GitRun(s.T(), worktreePath, "rev-parse", "--abbrev-ref", "HEAD"))
	s.Require().Empty(porcelain, "%s worktree must be clean (no conflict markers left behind)", label)
	s.Require().NotEqual("HEAD", head, "%s worktree must not be stuck on a detached mid-rebase HEAD", label)
}

// keptRebaseConflictOnChild produces a REAL conflicted state in the CHILD's own
// worktree (markers present, resolvable) via the kept-rebase path — mirroring
// TestWorktree_rebaseOntoParentConflictKeepsForResolve. It moves a child that
// conflicts with a second parent under that parent (reparent → moved-but-
// conflicting, clean), then POSTs rebase-onto-parent which KEEPS the conflicting
// rebase in progress. Returns the conflicted child's id. Unlike merge-into-
// parent (which aborts), this is the supported source of a live conflicted tree
// for the general /git/conflicts + /git/conflict-hunks endpoints.
func (s *ConflictsSuite) keptRebaseConflictOnChild() (childID string) {
	s.T().Helper()

	parentBID := s.Env.CreateChildWorkspace(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/conflict-parent-b", s.parentID,
	)
	parentBPath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, parentBID)
	kit.CommitFile(s.T(), parentBPath, "shared.txt", "parent-b version\n", "parent-b edit")
	parentBTip := kit.RevParse(s.T(), parentBPath, "HEAD")

	childID = s.Env.CreateChildWorkspace(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/conflict-rebase-child", s.parentID,
	)
	childPath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)
	kit.CommitFile(s.T(), childPath, "shared.txt", "child version\n", "child edit")

	watcher := s.Env.DialWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, childID)
	// Move under parentB: conflict → moved-but-conflicting, clean worktree.
	resp := s.Env.POST(s.T(), s.wsBase(childID)+"/reparent", map[string]any{"newParentId": parentBID})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["mergeConflicts"] == true
	})

	// Rebase onto parentB: KEEPS the conflicting rebase in the child's worktree.
	// The row is already pr-conflicts from the reparent above and the working
	// overlay re-broadcasts it when the async op begins, so wait on the op's
	// real outcome (the persisted fork point), not the status alone.
	resp2 := s.Env.POST(s.T(), s.wsBase(childID)+"/rebase-onto-parent", map[string]any{})
	kit.RequireStatus(s.T(), resp2, http.StatusAccepted)
	resp2.Body.Close()
	kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["status"] == "pr-conflicts" && m["forkPointSha"] == parentBTip
	})
	return childID
}

// TestConflicts_mergeDetectsConflict verifies a conflicting merge transitions
// the child to Status=pr-conflicts (broadcast over WS) AND — the H6/H7
// regression guard — leaves BOTH the parent and child worktrees clean (the
// in-progress merge is aborted; neither is ever left stuck).
func (s *ConflictsSuite) TestConflicts_mergeDetectsConflict() {
	childID := s.conflictSetup()
	// mergeConflict already asserts both worktrees are clean after the conflict.
	s.mergeConflict(childID)

	getResp := s.Env.GET(s.T(), s.wsBase(childID))
	kit.RequireStatus(s.T(), getResp, http.StatusOK)
	var reloaded map[string]any
	kit.DecodeEnvData(s.T(), getResp, &reloaded)
	s.Assert().Equal("pr-conflicts", reloaded["status"],
		"child workspace must be pr-conflicts after a conflicting merge")

	// Explicit, standalone H6/H7 guard: neither worktree carries conflict markers
	// or an in-progress op after a conflicting merge.
	childPath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)
	s.Assert().Empty(kit.TrimNewline(kit.GitRun(s.T(), s.parentPath, "status", "--porcelain")),
		"parent worktree must be clean after a conflicting merge (never bricked)")
	s.Assert().Empty(kit.TrimNewline(kit.GitRun(s.T(), childPath, "status", "--porcelain")),
		"child worktree must be clean after a conflicting merge")
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

// TestConflicts_conflictedFilesListsFile verifies the general /git/conflicts
// endpoint lists the conflicted file. The conflict SOURCE is a kept-rebase in
// the CHILD's own worktree (merge-into-parent no longer leaves markers anywhere
// — it aborts), so the endpoint is queried on the conflicted child.
func (s *ConflictsSuite) TestConflicts_conflictedFilesListsFile() {
	childID := s.keptRebaseConflictOnChild()

	conflictsResp := s.Env.GET(s.T(), s.wsBase(childID)+"/git/conflicts")
	kit.RequireStatus(s.T(), conflictsResp, http.StatusOK)

	var files []string
	kit.DecodeEnvData(s.T(), conflictsResp, &files)
	s.Require().NotEmpty(files)
	s.Assert().Contains(files, "shared.txt")
}

// TestConflicts_conflictHunksParsesThreeWayView verifies the general
// /git/conflict-hunks endpoint parses ours/theirs sections for the conflicted
// file. Same kept-rebase-on-child source as TestConflicts_conflictedFilesListsFile.
func (s *ConflictsSuite) TestConflicts_conflictHunksParsesThreeWayView() {
	childID := s.keptRebaseConflictOnChild()

	hunksResp := s.Env.GET(s.T(), s.wsBase(childID)+"/git/conflict-hunks?path=shared.txt")
	kit.RequireStatus(s.T(), hunksResp, http.StatusOK)

	var hunks []map[string]any
	kit.DecodeEnvData(s.T(), hunksResp, &hunks)
	s.Require().NotEmpty(hunks)
	s.Assert().NotEmpty(hunks[0]["ours"])
	s.Assert().NotEmpty(hunks[0]["theirs"])
}

// TestRegression_MergeConflictLeavesCleanParentResolvableViaRebase encodes the
// H6/H7 invariant that replaced the old "manually abort the stuck parent" flow:
// a conflicting merge-into-parent leaves the PARENT worktree already CLEAN (no
// in-progress op to abort — git status reports no files), the CHILD worktree
// clean, and the CHILD at pr-conflicts. No separate manual abort on the parent
// is needed or possible. (Resolvability of a real conflicted child via rebase-
// onto-parent — the kept-rebase-on-child path — is proven by
// TestConflicts_conflictedFilesListsFile and the worktree suite's
// TestWorktree_rebaseOntoParentConflictKeepsForResolve.)
func (s *ConflictsSuite) TestRegression_MergeConflictLeavesCleanParentResolvableViaRebase() {
	childID := s.conflictSetup()
	s.mergeConflict(childID) // also asserts both worktrees clean

	// The parent is already clean via the API's git status: zero files (the
	// conflicting merge was aborted internally; nothing left to abort manually).
	statusResp := s.Env.GET(s.T(), s.wsBase(s.parentID)+"/git/status")
	kit.RequireStatus(s.T(), statusResp, http.StatusOK)
	var status map[string]any
	kit.DecodeEnvData(s.T(), statusResp, &status)
	files, _ := status["files"].([]any)
	s.Assert().Empty(files, "parent must already be clean after a conflicting merge (never stuck)")

	// A manual abort on the now-clean parent finds nothing in progress — proving
	// there is no lingering half-merge to recover from (H6: the squash-conflict
	// brick would have left a non-abortable conflicted index here).
	abortResp := s.Env.POST(s.T(), s.wsBase(s.parentID)+"/git/operation/abort", nil)
	abortResp.Body.Close()
	s.Assert().NotEqual(http.StatusOK, abortResp.StatusCode,
		"no in-progress op should remain on the parent to abort")

	// The child stays pr-conflicts.
	childResp := s.Env.GET(s.T(), s.wsBase(childID))
	kit.RequireStatus(s.T(), childResp, http.StatusOK)
	var childWs map[string]any
	kit.DecodeEnvData(s.T(), childResp, &childWs)
	s.Assert().Equal("pr-conflicts", childWs["status"],
		"the conflict surfaces as the child's pr-conflicts state")
}
