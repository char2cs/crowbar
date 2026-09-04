//go:build integration

package worktree_test

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// WorktreeSuite holds integration tests for the worktree usecase over the
// chat-scoped, 202+WS worktree API (spec §3/§4/§5). The adopted main worktree
// (from ImportRepo) is locked (protected branch), so the suite's parent is an
// UNLOCKED child workspace forked from main; the tests' children fork from it.
type WorktreeSuite struct {
	kit.IntegrationSuite
	imported     kit.ImportedRepo
	parentID     string
	parentChatID string
	parentPath   string
}

// SetupTest imports a repo and creates an unlocked parent workspace (a child of
// the locked adopted main) before each test.
func (s *WorktreeSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "worktree", "")
	s.parentID, s.parentChatID = s.Env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/wt-base", "",
	)
	s.parentPath = s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.parentID)
}

// TestWorktreeSuite runs the WorktreeSuite integration tests.
func TestWorktreeSuite(t *testing.T) {
	suite.Run(t, new(WorktreeSuite))
}

// repoChatBase returns the route prefix for the worktree lifecycle verbs
// (lock/sync/merge-into-parent/reparent/rebase-onto-parent/retry-provision/
// detach-holder/branch), which stay keyed by CHAT id under the repo-scoped
// group rather than the flat /v0/chats/:chatId prefix (worktree/routes.go).
func (s *WorktreeSuite) repoChatBase(chatID string) string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/chats/" + chatID
}

// importBatchURL returns the batch branch-import route, the sole surviving
// import mount (spec §8 step 6 deleted its .../workspaces/import twin).
func (s *WorktreeSuite) importBatchURL() string {
	return "/v0/projects/" + s.imported.ProjectID + "/repos/" + s.imported.RepoID + "/chats/import-batch"
}

// createChild creates a child workspace under the suite's parent and returns
// its id, the id of the chat that owns it, and its on-disk worktree path.
func (s *WorktreeSuite) createChild(branch string) (wsID, chatID, path string) {
	s.T().Helper()
	return s.createChildUnder(s.parentID, branch)
}

// createChildUnder creates a child workspace under parentID and returns its
// id, its owning chat's id, and its on-disk worktree path.
func (s *WorktreeSuite) createChildUnder(parentID, branch string) (wsID, chatID, path string) {
	s.T().Helper()
	wsID, chatID = s.Env.CreateWorkspaceWithChat(s.T(), s.imported.ProjectID, s.imported.RepoID, branch, parentID)
	return wsID, chatID, s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, wsID)
}

// getWorkspace fetches chatID's detail DTO and projects it down to the flat
// workspace shape the tests assert on (kit.WorktreeFrame's REST twin) —
// GET .../chats/:id is the only surviving single-worktree read now that the
// :wsId-keyed GET is gone (spec §8 step 6).
func (s *WorktreeSuite) getWorkspace(chatID string) map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), s.repoChatBase(chatID))
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var raw map[string]any
	kit.DecodeEnvData(s.T(), resp, &raw)
	return kit.WorktreeFrame(raw)
}

// defaultBranch returns the repo's default branch as the daemon records it —
// read from the API rather than from the repo's HEAD, which Crowbar may have
// detached to free the branch for a managed worktree.
func (s *WorktreeSuite) defaultBranch() string {
	s.T().Helper()
	resp := s.Env.GET(s.T(), "/v0/projects/"+s.imported.ProjectID+"/repos")
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var repos []map[string]any
	kit.DecodeEnvData(s.T(), resp, &repos)
	for _, r := range repos {
		if r["id"] == s.imported.RepoID {
			branch, _ := r["defaultBranch"].(string)
			s.Require().NotEmpty(branch, "the imported repo must report a default branch")
			return branch
		}
	}
	s.Require().Fail("imported repo not found in the repo list")
	return ""
}

// workspaceChats returns the repo's worktree-owning chats projected to the
// flat workspace shape, keyed by workspace id — the read-model replacement
// for the deleted workspace list (kit.Env.WorktreeChats).
func (s *WorktreeSuite) workspaceChats() map[string]map[string]any {
	s.T().Helper()
	return s.Env.WorktreeChats(s.T(), s.imported.ProjectID, s.imported.RepoID)
}

// mergeIntoParent triggers a child→parent merge (202) and blocks until the child
// WorkspaceDTO reaches the given terminal forkPointSha over WS (the merge
// updates the child's fork point to the new parent tip). Returns the parent's
// post-merge HEAD sha. chatID is the id of the chat that owns childID.
func (s *WorktreeSuite) mergeIntoParent(childID, chatID, strategy string) string {
	s.T().Helper()
	// Capture the child's pre-merge fork point so we can wait for the merge to
	// CHANGE it (a successful merge updates the child fork point to the new parent
	// tip; a fresh child's fork point is non-empty from creation, so "non-empty"
	// alone would race the connect snapshot).
	before := s.getWorkspace(chatID)
	preMergeFork, _ := before["forkPointSha"].(string)

	watcher := s.Env.DialChat(s.T(), chatID)
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
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

	childID, chatID, worktreePath := s.createChild("feature/create-test")

	s.Assert().True(kit.DirExists(t, worktreePath), "child worktree path must exist on disk")
	s.Assert().True(kit.BranchExists(t, s.imported.RepoPath, "feature/create-test"),
		"branch feature/create-test must exist after CreateChild")

	child := s.getWorkspace(chatID)
	s.Assert().Equal(s.parentID, child["parentId"])

	kit.AssertWorkspaceConsistency(t, s.Env, s.imported.RepoPath, childID)
}

// TestWorktree_importCreatesWorkspacesForBranches exercises the batch import
// endpoint end-to-end over the real daemon: POST .../chats/import-batch with a
// set of branches returns 202 and each branch's worktree is provisioned and
// linked to its own owning chat. The fixture repo has no GitHub PRs, so this
// pins the no-PR path (each branch forks from the default) — the PR-parenting
// logic is unit-integration-tested against a stubbed PR graph in the worktree
// usecase.
//
// It blocks on each chat's "workspace_set" lifecycle frame rather than a
// worktree_state one carrying the branch: the atomic mint-then-attach sequence
// batch import shares with a single chat create (hierarchyOwningChats.
// AttachOwningWorkspace, container.go — the same pattern
// chat.SpawnChatWithOwnWorktree uses) mints the owning chat BEFORE the
// workspace exists, so the workspace's OWN creation-time push resolves no
// owning chat and is dropped (pushChatWorktree, container.go); attaching the
// chat afterward only emits the terse chat-lifecycle event, never a re-push of
// the linked workspace's git state. So "workspace_set" — not a branch-bearing
// frame — is the only reliable per-branch completion signal this route
// produces; each branch's actual state is then asserted directly on disk.
func (s *WorktreeSuite) TestWorktree_importCreatesWorkspacesForBranches() {
	t := s.T()
	watcher := s.Env.DialRepoChats(t, s.imported.ProjectID, s.imported.RepoID)

	resp := s.Env.POST(t, s.importBatchURL(), map[string]any{
		"branches": []string{"import/alpha", "import/beta"},
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	linked := map[string]bool{}
	watcher.ReadUntil(t, 20*time.Second, func(m map[string]any) bool {
		if m["repoId"] != s.imported.RepoID || m["kind"] != "workspace_set" {
			return false
		}
		if wsID, _ := m["workspaceId"].(string); wsID != "" {
			linked[wsID] = true
		}
		return len(linked) >= 2
	})

	s.Assert().True(kit.BranchExists(t, s.imported.RepoPath, "import/alpha"),
		"import must create branch import/alpha")
	s.Assert().True(kit.BranchExists(t, s.imported.RepoPath, "import/beta"),
		"import must create branch import/beta")
}

// TestWorktree_importHeldBranchYieldsPlaceholder is the black-box regression for
// the confirmed production hang: importing a branch that a worktree OUTSIDE
// Crowbar already has checked out produced nothing at all. git refuses to check
// a branch out twice, CreateChild failed, the batch swallowed it and returned
// success, and the handler's error channel is a no-op without a workspace id —
// so no frame ever reached this stream and the client's optimistic import row,
// which is cleared only by a workspace for that branch, spun forever with
// nothing surfaced.
//
// The contract asserted here is that EVERY imported branch produces a row a
// client can learn of: an unmaterialisable one arrives as a placeholder (no
// localPath, carrying the holder path) that the client can explain, retry and
// detach.
//
// The row is confirmed over REST rather than a branch-bearing WS frame, for
// the same reason TestWorktree_importCreatesWorkspacesForBranches gives:
// batch import mints the owning chat before the workspace exists and only
// broadcasts the terse "workspace_set" link afterward, never the linked
// workspace's own git state (container.go's pushChatWorktree needs the chat
// to already be attached, and nothing re-pushes once it is). "workspace_set"
// firing at all is still the regression's core proof — the old bug was that
// a held branch produced NO frame whatsoever and the client's row spun
// forever; here the chat is reliably linked, and its placeholder shape is
// then read off the row directly.
func (s *WorktreeSuite) TestWorktree_importHeldBranchYieldsPlaceholder() {
	t := s.T()

	// A live worktree outside crowbar home holds the branch — the production
	// trigger was another tool's worktree under ~/.superconductor.
	holderPath := filepath.Join(t.TempDir(), "external-holder")
	kit.GitRun(t, s.imported.RepoPath, "worktree", "add", "-b", "import/held", holderPath)

	watcher := s.Env.DialRepoChats(t, s.imported.ProjectID, s.imported.RepoID)
	resp := s.Env.POST(t, s.importBatchURL(), map[string]any{
		"branches": []string{"import/held"},
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	var chatID string
	watcher.ReadUntil(t, 20*time.Second, func(m map[string]any) bool {
		if m["repoId"] != s.imported.RepoID || m["kind"] != "workspace_set" {
			return false
		}
		if wsID, _ := m["workspaceId"].(string); wsID != "" {
			chatID, _ = m["chatId"].(string)
			return true
		}
		return false
	})
	s.Require().NotEmpty(chatID, "import must link the held branch's workspace to an owning chat")
	s.Env.Quiesce()

	placeholder := s.getWorkspace(chatID)
	s.Require().Equal("import/held", placeholder["branch"],
		"the linked chat must own the import/held branch, not some other row the batch created")

	s.Assert().Empty(placeholder["localPath"],
		"a held branch has no managed worktree — the empty path IS the placeholder signal")
	s.Assert().NotEmpty(placeholder["heldByPath"],
		"the holder path is what the placeholder toast and the Detach… modal render")
	s.Assert().NotEqual("locked", placeholder["status"],
		"an imported feature branch must not be locked; locked survives provisioning "+
			"and would block merge/rename/delete forever")
}

// TestWorktree_importDefaultBranchIsRefusedSynchronously pins the other silent
// hang: the import chain walk terminates AT the default branch, so a request
// naming it created nothing while still answering 202 — leaving the caller's
// optimistic row waiting on a workspace that was never coming. It must be
// refused on the request path, where the caller can surface it.
func (s *WorktreeSuite) TestWorktree_importDefaultBranchIsRefusedSynchronously() {
	t := s.T()

	resp := s.Env.POST(t, s.importBatchURL(), map[string]any{
		"branches": []string{s.defaultBranch()},
	})
	defer resp.Body.Close()
	kit.RequireStatus(t, resp, http.StatusConflict)
}

// TestWorktree_mergeStrategyMerge verifies the fast-forward merge advances the parent.
func (s *WorktreeSuite) TestWorktree_mergeStrategyMerge() {
	t := s.T()

	childID, chatID, worktreePath := s.createChild("feature/merge-test")
	kit.CommitFile(t, worktreePath, "merge.txt", "child change\n", "child commit")

	parentTip := s.mergeIntoParent(childID, chatID, "merge")
	s.Assert().Equal(parentTip, kit.RevParse(t, s.parentPath, "HEAD"),
		"parent HEAD must match the merged tip")

	reloaded := s.getWorkspace(chatID)
	s.Assert().Equal(parentTip, reloaded["forkPointSha"])
}

// TestWorktree_mergeConflictsFalseForCleanChild verifies a child whose changes
// fold cleanly into its parent reports mergeConflicts:false.
func (s *WorktreeSuite) TestWorktree_mergeConflictsFalseForCleanChild() {
	t := s.T()
	_, chatID, worktreePath := s.createChild("feature/clean-merge")
	kit.CommitFile(t, worktreePath, "clean.txt", "clean\n", "clean commit")

	ws := s.getWorkspace(chatID)
	s.Assert().Equal(false, ws["mergeConflicts"], "a cleanly-mergeable child must report mergeConflicts:false")
	s.Assert().Equal(true, ws["canMergeLocally"], "structurally mergeable")
}

// TestWorktree_mergeDeleteSourceRemovesChild verifies merge-into-parent with
// deleteSource:true folds the child branch into the parent AND removes the
// now-merged child workspace (worktree on disk, branch, record), emitting a
// deleted tombstone. The parent must still advance to the merged tip.
func (s *WorktreeSuite) TestWorktree_mergeDeleteSourceRemovesChild() {
	t := s.T()

	parentTipBefore := kit.RevParse(t, s.parentPath, "HEAD")

	childID, chatID, worktreePath := s.createChild("feature/merge-delete-source")
	kit.CommitFile(t, worktreePath, "md.txt", "child change\n", "child commit")

	watcher := s.Env.DialChat(t, chatID)
	resp := s.Env.POST(t, s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
		"strategy":     "merge",
		"deleteSource": true,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Merge runs, then the child is cascade-deleted. The workspace delete's own
	// "deleted" worktree_state frame is NOT a reliable signal to wait on: it
	// races the SAME delete's registered dependent-forget reactor
	// (container.go's forgetDependents → forgetAgentChats, wired on every
	// workspace delete via RegisterDeleteReactor), which forgets the child's
	// owning chat asynchronously — and pushChatWorktree drops the tombstone
	// outright once that chat no longer resolves (a workspace with no owning
	// chat pushes nothing). That reactor's OWN forget, in contrast, always
	// fires and always broadcasts the owning chat's "deleted" lifecycle event,
	// so THAT is the deterministic completion signal here.
	watcher.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		return m["chatId"] == chatID && m["kind"] == "deleted"
	})

	s.Assert().NotEqual(parentTipBefore, kit.RevParse(t, s.parentPath, "HEAD"),
		"merge must advance the parent before the child is removed")
	s.Assert().False(kit.DirExists(t, worktreePath), "child worktree must be removed on disk")
	s.Assert().False(kit.BranchExists(t, s.imported.RepoPath, "feature/merge-delete-source"),
		"child branch must be force-deleted")
	_, stillListed := s.workspaceChats()[childID]
	s.Assert().False(stillListed, "deleted child must not appear in the workspace list")
}

// TestWorktree_mergeDeleteSourceKeepsNonLeafChild verifies deleteSource:true does
// NOT remove a merged child that still has its own child workspace — cascade-
// deleting it would destroy the grandchild's unmerged work. The merge still lands.
func (s *WorktreeSuite) TestWorktree_mergeDeleteSourceKeepsNonLeafChild() {
	t := s.T()

	parentTipBefore := kit.RevParse(t, s.parentPath, "HEAD")

	childID, chatID, childPath := s.createChild("feature/non-leaf-parent")
	kit.CommitFile(t, childPath, "nl.txt", "child\n", "child commit")
	preMergeFork, _ := s.getWorkspace(chatID)["forkPointSha"].(string)
	grandchildID, _, _ := s.createChildUnder(childID, "feature/non-leaf-grandchild")

	watcher := s.Env.DialChat(t, chatID)
	resp := s.Env.POST(t, s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
		"strategy":     "merge",
		"deleteSource": true,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Wait for the merge to finalize (child fork point advances). The non-leaf
	// child is then kept — no deleted tombstone follows.
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		fp, _ := m["forkPointSha"].(string)
		return fp != "" && fp != preMergeFork
	})
	s.Assert().NotEqual(parentTipBefore, kit.RevParse(t, s.parentPath, "HEAD"), "merge must advance the parent")

	chats := s.workspaceChats()
	s.Assert().Contains(chats, childID, "non-leaf merged child must be kept (deleting it would cascade the grandchild)")
	s.Assert().Contains(chats, grandchildID, "grandchild must survive")
}

// TestWorktree_mergeStrategySquash verifies squash merge collapses child commits.
func (s *WorktreeSuite) TestWorktree_mergeStrategySquash() {
	t := s.T()

	parentTipBefore := kit.RevParse(t, s.parentPath, "HEAD")

	childID, chatID, worktreePath := s.createChild("feature/squash-test")
	kit.CommitFile(t, worktreePath, "a.txt", "a\n", "commit a")
	kit.CommitFile(t, worktreePath, "b.txt", "b\n", "commit b")

	s.mergeIntoParent(childID, chatID, "squash")

	parentTip := kit.RevParse(t, s.parentPath, "HEAD")
	s.Assert().NotEqual(parentTipBefore, parentTip, "squash must advance parent")

	parentMinus1 := kit.RevParse(t, s.parentPath, "HEAD~1")
	s.Assert().Equal(parentTipBefore, parentMinus1, "squash must produce exactly one new commit on parent")
}

// TestWorktree_mergeStrategyRebase verifies rebase + FF merge rewrites child SHA.
func (s *WorktreeSuite) TestWorktree_mergeStrategyRebase() {
	t := s.T()

	childID, chatID, worktreePath := s.createChild("feature/rebase-test")
	kit.CommitFile(t, worktreePath, "child.txt", "child\n", "child commit")
	childTipBefore := kit.RevParse(t, worktreePath, "HEAD")

	kit.CommitFile(t, s.parentPath, "parent.txt", "parent\n", "parent commit")

	parentTip := s.mergeIntoParent(childID, chatID, "rebase")

	childTipAfter := kit.RevParse(t, worktreePath, "HEAD")
	s.Assert().NotEqual(childTipBefore, childTipAfter, "rebase must rewrite child SHA")
	s.Assert().Equal(parentTip, childTipAfter, "FF-merge means parent == child tip")
}

// TestWorktree_reparent verifies leaf re-parenting replays child commits onto new parent.
func (s *WorktreeSuite) TestWorktree_reparent() {
	t := s.T()

	parentBID, _, parentBPath := s.createChild("feature/parent-b")
	kit.CommitFile(t, parentBPath, "pb.txt", "parent-b\n", "parent-b commit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	childID, childChatID, childPath := s.createChild("feature/child")
	kit.CommitFile(t, childPath, "child.txt", "child\n", "child commit")

	watcher := s.Env.DialChat(t, childChatID)
	// newParentId still names a WORKSPACE (git lineage), not a chat — see
	// ChatReparent's own doc.
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["forkPointSha"] == parentBTip
	})

	s.Assert().True(kit.FileExists(t, childPath+"/child.txt"), "child's own commit must survive reparent")
	s.Assert().True(kit.FileExists(t, childPath+"/pb.txt"), "new parent's history must be in child branch")

	reloaded := s.getWorkspace(childChatID)
	s.Assert().Equal(parentBID, reloaded["parentId"])
	s.Assert().Equal(parentBTip, reloaded["forkPointSha"])
}

// TestWorktree_reparentWithChildrenRejected verifies the has_children guard. The
// guard runs in the background (00 §4: reparent is 202+async), so the rejection
// surfaces as a lastError on the workspace WS rather than a synchronous 409. The
// child's git history must be untouched.
// TestWorktree_reparentConflictMovesButStaysClean verifies the try-then-warn
// model: a reparent whose rebase conflicts MOVES the child under the new parent
// anyway, but ABORTS the rebase so the worktree stays clean (never stuck), and
// the predicted-conflict flag lights up.
func (s *WorktreeSuite) TestWorktree_reparentConflictMovesButStaysClean() {
	t := s.T()

	parentBID, _, parentBPath := s.createChild("feature/parent-b-conflict")
	kit.CommitFile(t, parentBPath, "shared.txt", "parent-b version\n", "parent-b edit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	childID, childChatID, childPath := s.createChild("feature/child-conflict")
	kit.CommitFile(t, childPath, "shared.txt", "child version\n", "child edit") // same file, diverging

	watcher := s.Env.DialChat(t, childChatID)
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{"newParentId": parentBID})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// Moved under parentB AND flagged as conflicting.
	moved := kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["mergeConflicts"] == true
	})
	// Moved on paper, NOT cleanly integrated: fork point is the merge-base, not
	// the new parent's tip (which a clean rebase would have produced).
	s.Assert().NotEqual(parentBTip, moved["forkPointSha"], "a conflicted reparent must not finalize onto the new tip")

	// The worktree is CLEAN, not stuck mid-rebase.
	s.Assert().Equal("feature/child-conflict",
		kit.TrimNewline(kit.GitRun(t, childPath, "rev-parse", "--abbrev-ref", "HEAD")),
		"worktree must be on its branch, not a detached mid-rebase HEAD")
	s.Assert().Empty(kit.TrimNewline(kit.GitRun(t, childPath, "status", "--porcelain")),
		"worktree must be clean (no conflict markers, no in-progress rebase)")
	s.Assert().True(kit.FileExists(t, childPath+"/shared.txt"), "child's own work survives")
}

// TestWorktree_rebaseOntoParentConflictKeepsForResolve verifies the user-initiated
// "finish the move": rebasing a moved-but-conflicting child onto its parent KEEPS
// the conflicting rebase (status pr-conflicts) for the standard resolve flow, and
// persists the intended fork point up front so the branch reads correctly once
// resolved.
func (s *WorktreeSuite) TestWorktree_rebaseOntoParentConflictKeepsForResolve() {
	t := s.T()

	parentBID, _, parentBPath := s.createChild("feature/rop-parent")
	kit.CommitFile(t, parentBPath, "shared.txt", "parent version\n", "parent edit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	childID, childChatID, childPath := s.createChild("feature/rop-child")
	kit.CommitFile(t, childPath, "shared.txt", "child version\n", "child edit")

	watcher := s.Env.DialChat(t, childChatID)
	// Move it under parentB: conflict → moved-but-conflicting, clean worktree.
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{"newParentId": parentBID})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["mergeConflicts"] == true
	})

	// User clicks "Rebase onto parent" → keep the conflict for resolution.
	// The row is already pr-conflicts from the reparent above, and the working
	// overlay re-broadcasts the current row when the async op begins — so wait
	// on the op's real outcome (the persisted fork point), not the status alone.
	resp2 := s.Env.POST(t, s.repoChatBase(childChatID)+"/rebase-onto-parent", map[string]any{})
	kit.RequireStatus(t, resp2, http.StatusAccepted)
	resp2.Body.Close()
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["status"] == "pr-conflicts" && m["forkPointSha"] == parentBTip
	})

	got := s.getWorkspace(childChatID)
	s.Assert().Equal(parentBTip, got["forkPointSha"], "the intended fork point is persisted up front")
	s.Assert().Equal("HEAD",
		kit.TrimNewline(kit.GitRun(t, childPath, "rev-parse", "--abbrev-ref", "HEAD")),
		"the rebase is kept in progress (detached HEAD) for the user to resolve")
}

// TestWorktree_asyncOpBroadcastsWorkingOverlay pins the working-overlay
// contract on the wire: accepting an async workspace mutation immediately
// broadcasts the row with working=true (the tree spinner starts with the 202),
// and the op's completion broadcasts working=false so the spinner always
// resolves.
func (s *WorktreeSuite) TestWorktree_asyncOpBroadcastsWorkingOverlay() {
	t := s.T()

	childID, chatID, _ := s.createChild("feature/working-overlay")
	watcher := s.Env.DialChat(t, chatID)

	resp := s.Env.POST(t, s.repoChatBase(chatID)+"/sync", map[string]any{})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["working"] == true
	})
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["working"] == false
	})
}

func (s *WorktreeSuite) TestWorktree_reparentWithChildrenRejected() {
	t := s.T()

	parentBID, _, _ := s.createChild("feature/parent-b2")

	childID, childChatID, childPath := s.createChild("feature/child-rejected")
	kit.CommitFile(t, childPath, "c.txt", "c\n", "child commit")
	childTipBefore := kit.RevParse(t, childPath, "HEAD")

	// Create a grandchild of `child` — Reparent must reject when child has children.
	s.createChildUnder(childID, "feature/grandchild-rejected")

	watcher := s.Env.DialChat(t, childChatID)
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	kit.WaitForWorkspaceLastError(t, watcher, childID, 5*time.Second)

	childTipAfterRejection := kit.RevParse(t, childPath, "HEAD")
	s.Assert().Equal(childTipBefore, childTipAfterRejection, "rejected reparent must not mutate git")

	// The parent must NOT have changed (rejected reparent).
	reloaded := s.getWorkspace(childChatID)
	s.Assert().Equal(s.parentID, reloaded["parentId"], "rejected reparent must keep the original parent")
}

// TestRegression_ReparentOntoSelfRejected guards a corruption found in manual
// testing: reparenting a workspace onto ITSELF was unguarded, producing
// parentId == id. That self-loop detached the node in the tree AND made it
// permanently unreparentable (the leaf check counted the node as its own child).
// The guard now rejects it; the rejection surfaces as a lastError (reparent is
// 202+async) and the workspace's parent and git history are untouched.
func (s *WorktreeSuite) TestRegression_ReparentOntoSelfRejected() {
	t := s.T()

	childID, childChatID, childPath := s.createChild("feature/self-parent")
	kit.CommitFile(t, childPath, "c.txt", "c\n", "child commit")
	childTipBefore := kit.RevParse(t, childPath, "HEAD")

	watcher := s.Env.DialChat(t, childChatID)
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{
		"newParentId": childID, // onto itself
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	le := kit.WaitForWorkspaceLastError(t, watcher, childID, 5*time.Second)
	s.Assert().Contains(le, "itself", "self-parent must be rejected with a clear error")

	reloaded := s.getWorkspace(childChatID)
	s.Assert().Equal(s.parentID, reloaded["parentId"], "self-parent must not change the parent")
	s.Assert().NotEqual(childID, reloaded["parentId"], "workspace must never become its own parent")
	s.Assert().Equal(childTipBefore, kit.RevParse(t, childPath, "HEAD"), "rejected self-parent must not mutate git")
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

	parentID, parentChatID, _ := s.createChild("feature/elig-parent")

	// Idle parent → a freshly created child can merge locally onto the parent
	// branch. A bare create mints no owning chat, and pushChatWorktree pushes
	// NOTHING without one (container.go) — so this reads the freshly computed
	// eligibility over REST rather than waiting on a WS frame that cannot arrive
	// for a chat-less row.
	_, childChatID := s.Env.CreateWorkspaceWithChat(t, s.imported.ProjectID, s.imported.RepoID, "feature/elig-child", parentID)
	created := s.getWorkspace(childChatID)
	can, _ := created["canMergeLocally"].(bool)
	s.Assert().True(can && created["parentBranch"] == "feature/elig-parent",
		"an idle parent must make a freshly created child eligible to merge locally")

	// Lock the parent → the child's eligibility flips to false.
	parentWatcher := s.Env.DialChat(t, parentChatID)
	s.Env.PushProviderState(t, parentID, kit.ProviderState{Protected: true})
	kit.WaitForWorkspaceState(t, parentWatcher, parentID, "locked", 5*time.Second)

	child := s.getWorkspace(childChatID)
	can2, _ := child["canMergeLocally"].(bool)
	s.Assert().False(can2, "a locked parent must make the child ineligible to merge locally")
}

// TestWorktree_deleteCascadeSkipsLockedChild verifies cascade delete preserves a
// locked child. The child is locked via the provider seam (Protected:true →
// Status=locked) since locking is status-based now (spec §5), not a create-body
// flag. Delete now runs through DELETE .../chats/:id (the chat owning the root
// workspace): DeleteChat reaps ITS OWN worktree through the same
// hierarchy.DeleteCascade the old :wsId route called (worktreeChildCreator.
// DiscardChildWorkspace, container.go), so the workspace-lineage cascade and its
// skip-locked guard are unchanged — only the address moved.
func (s *WorktreeSuite) TestWorktree_deleteCascadeSkipsLockedChild() {
	t := s.T()

	rootID, rootChatID, _ := s.createChild("feature/root-cascade")
	lockedID, lockedChatID, _ := s.createChildUnder(rootID, "feature/locked-branch")

	// Lock the child via a protected provider state.
	lockWatcher := s.Env.DialChat(t, lockedChatID)
	s.Env.PushProviderState(t, lockedID, kit.ProviderState{Protected: true})
	kit.WaitForWorkspaceState(t, lockWatcher, lockedID, "locked", 5*time.Second)

	rootWatcher := s.Env.DialChat(t, rootChatID)
	resp := s.Env.DELETE(t, s.repoChatBase(rootChatID))
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()
	// DeleteChat reaps rootID's worktree AND purges rootChatID itself in the SAME
	// request (chatFolderUsecase.DeleteChat: reap-then-purge). The two race on
	// the wire: pushChatWorktree needs the owning chat to still resolve at the
	// moment the workspace's own "deleted" worktree_state frame is built
	// (container.go — a workspace with no resolved owning chat pushes NOTHING),
	// and that chat can already be gone by then — so the worktree's own
	// tombstone is not a reliable signal to wait on here. The chat's own
	// "deleted" lifecycle frame is, since it is what this same purge
	// unconditionally emits.
	rootWatcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["chatId"] == rootChatID && m["kind"] == "deleted"
	})

	// The frame above proves the delete command landed; QuiesceReactors then
	// joins the detached purge reactor (worktree removal, Forget cascades) and
	// folds the projections it dispatches, so the list below is read at the
	// CONVERGED state rather than sampled hoping to catch it.
	s.Env.QuiesceReactors()

	chats := s.workspaceChats()
	s.Require().Contains(chats, lockedID, "cascade delete must KEEP the locked child")
	s.Require().NotContains(chats, rootID, "cascade delete must reap the unlocked root")
}
