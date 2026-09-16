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
	imported     kit.ImportedRepo
	parentID     string
	parentChatID string
	parentPath   string
}

// SetupTest imports a repo and creates an UNLOCKED parent workspace (a child of
// the locked adopted main) for each test case. Merging into a locked parent is
// rejected by the guard, so the parent must be an unlocked feature branch.
func (s *ConflictsSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "conflicts", "")
	s.parentID, s.parentChatID = s.Env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/conflicts-base", "",
	)
	s.parentPath = s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.parentID)
}

// repoChatBase returns the route prefix for the worktree lifecycle verbs
// (merge-into-parent/reparent/rebase-onto-parent/...), keyed by CHAT id under
// the repo-scoped group (worktree/routes.go).
func (s *ConflictsSuite) repoChatBase(chatID string) string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/chats/" + chatID
}

// chatBase returns the FLAT chat-scoped route prefix git/review mount on
// (spec §8 step 6 deleted their :wsId-keyed twins).
func (s *ConflictsSuite) chatBase(chatID string) string {
	return "/v0/chats/" + chatID
}

// createChildWithChat creates a child workspace under parentID and returns its
// id, its owning chat's id, and its on-disk worktree path.
func (s *ConflictsSuite) createChildWithChat(parentID, branch string) (wsID, chatID, path string) {
	s.T().Helper()
	wsID, chatID = s.Env.CreateWorkspaceWithChat(s.T(), s.imported.ProjectID, s.imported.RepoID, branch, parentID)
	return wsID, chatID, s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, wsID)
}

// getWorkspace fetches chatID's detail DTO and projects it down to the flat
// workspace shape the tests assert on (kit.WorktreeFrame's REST twin) — the
// only surviving single-worktree read now that the :wsId-keyed GET is gone.
func (s *ConflictsSuite) getWorkspace(chatID string) map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), s.repoChatBase(chatID))
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var raw map[string]any
	kit.DecodeEnvData(s.T(), resp, &raw)
	return kit.WorktreeFrame(raw)
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
// parent to produce a merge conflict. Returns the child workspace id and its
// owning chat's id.
func (s *ConflictsSuite) conflictSetup() (childID, chatID string) {
	s.T().Helper()

	// Commit the base version of shared.txt on the (unlocked) parent worktree.
	kit.CommitFile(
		s.T(),
		s.parentPath,
		"shared.txt",
		"base line\n",
		"base",
	)

	childID, chatID, childWorktreePath := s.createChildWithChat(s.parentID, "feature/conflict")

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
	return childID, chatID
}

// mergeConflict triggers a conflicting child→parent merge (202) and blocks until
// the async merge has run and settled into the try-then-warn end state: the
// child reaches Status=pr-conflicts AND BOTH worktrees are clean (the merge was
// aborted, never left stuck). It dials the child's chat and waits for the
// post-merge pr-conflicts frame, then asserts cleanliness directly.
//
// It cannot simply key on the child reaching Status=pr-conflicts via REST: that
// status is also produced up front by the merge-tree prediction overlay (a child
// with diverging edits reads pr-conflicts before any merge is attempted). The
// reliable post-condition is the worktrees having been aborted clean — so the
// helper waits on the WS frame after the merge and then verifies both worktrees.
func (s *ConflictsSuite) mergeConflict(childID, chatID string) {
	s.T().Helper()
	childPath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)

	watcher := s.Env.DialChat(s.T(), chatID)
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/merge-into-parent", map[string]string{
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
// rebase in progress. Returns the conflicted child's id and its owning chat's
// id. Unlike merge-into-parent (which aborts), this is the supported source of
// a live conflicted tree for the general /git/conflicts + /git/conflict-hunks
// endpoints.
func (s *ConflictsSuite) keptRebaseConflictOnChild() (childID, chatID string) {
	s.T().Helper()

	parentBID, _, parentBPath := s.createChildWithChat(s.parentID, "feature/conflict-parent-b")
	kit.CommitFile(s.T(), parentBPath, "shared.txt", "parent-b version\n", "parent-b edit")
	parentBTip := kit.RevParse(s.T(), parentBPath, "HEAD")

	childID, chatID, childPath := s.createChildWithChat(s.parentID, "feature/conflict-rebase-child")
	kit.CommitFile(s.T(), childPath, "shared.txt", "child version\n", "child edit")

	watcher := s.Env.DialChat(s.T(), chatID)
	// Move under parentB: conflict → moved-but-conflicting, clean worktree.
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/reparent", map[string]any{"newParentId": parentBID})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["mergeConflicts"] == true
	})

	// Rebase onto parentB: KEEPS the conflicting rebase in the child's worktree.
	// The row is already pr-conflicts from the reparent above and the working
	// overlay re-broadcasts it when the async op begins, so wait on the op's
	// real outcome (the persisted fork point), not the status alone.
	resp2 := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/rebase-onto-parent", map[string]any{})
	kit.RequireStatus(s.T(), resp2, http.StatusAccepted)
	resp2.Body.Close()
	kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["status"] == "pr-conflicts" && m["forkPointSha"] == parentBTip
	})
	return childID, chatID
}

// TestConflicts_mergeDetectsConflict verifies a conflicting merge transitions
// the child to Status=pr-conflicts (broadcast over WS) AND — the H6/H7
// regression guard — leaves BOTH the parent and child worktrees clean (the
// in-progress merge is aborted; neither is ever left stuck).
func (s *ConflictsSuite) TestConflicts_mergeDetectsConflict() {
	childID, chatID := s.conflictSetup()
	// mergeConflict already asserts both worktrees are clean after the conflict.
	s.mergeConflict(childID, chatID)

	reloaded := s.getWorkspace(chatID)
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
	_, chatID := s.conflictSetup() // diverging edits to shared.txt on child + parent; no merge

	ws := s.getWorkspace(chatID)

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
	childID, chatID := s.conflictSetup() // conflict between child + parent, no merge

	watcher := s.Env.DialChat(s.T(), chatID)
	resp := s.Env.PATCH(s.T(), s.chatBase(chatID)+"/review", map[string]any{"mergeStrategy": "squash"})
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
	childID, chatID := s.conflictSetup()

	watcher := s.Env.DialChat(s.T(), chatID)
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
		"strategy":     "merge",
		"deleteSource": true,
	})
	kit.RequireStatus(s.T(), resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspaceState(s.T(), watcher, childID, "pr-conflicts", 5*time.Second)

	// Despite deleteSource:true, the conflicted child must survive.
	getResp := s.Env.GET(s.T(), s.repoChatBase(chatID))
	kit.RequireStatus(s.T(), getResp, http.StatusOK)
}

// TestConflicts_conflictedFilesListsFile verifies the general /git/conflicts
// endpoint lists the conflicted file. The conflict SOURCE is a kept-rebase in
// the CHILD's own worktree (merge-into-parent no longer leaves markers anywhere
// — it aborts), so the endpoint is queried on the conflicted child's chat.
func (s *ConflictsSuite) TestConflicts_conflictedFilesListsFile() {
	_, chatID := s.keptRebaseConflictOnChild()

	conflictsResp := s.Env.GET(s.T(), s.chatBase(chatID)+"/git/conflicts")
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
	_, chatID := s.keptRebaseConflictOnChild()

	hunksResp := s.Env.GET(s.T(), s.chatBase(chatID)+"/git/conflict-hunks?path=shared.txt")
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
	childID, chatID := s.conflictSetup()
	s.mergeConflict(childID, chatID) // also asserts both worktrees clean

	// The parent is already clean via the API's git status: zero files (the
	// conflicting merge was aborted internally; nothing left to abort manually).
	statusResp := s.Env.GET(s.T(), s.chatBase(s.parentChatID)+"/git/status")
	kit.RequireStatus(s.T(), statusResp, http.StatusOK)
	var status map[string]any
	kit.DecodeEnvData(s.T(), statusResp, &status)
	files, _ := status["files"].([]any)
	s.Assert().Empty(files, "parent must already be clean after a conflicting merge (never stuck)")

	// A manual abort on the now-clean parent finds nothing in progress — proving
	// there is no lingering half-merge to recover from (H6: the squash-conflict
	// brick would have left a non-abortable conflicted index here).
	abortResp := s.Env.POST(s.T(), s.chatBase(s.parentChatID)+"/git/operation/abort", nil)
	abortResp.Body.Close()
	s.Assert().NotEqual(http.StatusOK, abortResp.StatusCode,
		"no in-progress op should remain on the parent to abort")

	// The child stays pr-conflicts.
	reloaded := s.getWorkspace(chatID)
	s.Assert().Equal("pr-conflicts", reloaded["status"],
		"the conflict surfaces as the child's pr-conflicts state")
}
