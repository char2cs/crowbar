//go:build integration

package worktree_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

type WorktreeSuite struct {
	kit.IntegrationSuite
	imported     kit.ImportedRepo
	parentID     string
	parentChatID string
	parentPath   string
}

func (s *WorktreeSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "worktree", "")
	s.parentID, s.parentChatID = s.Env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/wt-base", "",
	)
	s.parentPath = s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.parentID)
}

func TestWorktreeSuite(t *testing.T) {
	suite.Run(t, new(WorktreeSuite))
}

func (s *WorktreeSuite) repoChatBase(chatID string) string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/chats/" + chatID
}

func (s *WorktreeSuite) createChild(branch string) (wsID, chatID, path string) {
	s.T().Helper()
	return s.createChildUnder(s.parentID, branch)
}

func (s *WorktreeSuite) createChildUnder(parentID, branch string) (wsID, chatID, path string) {
	s.T().Helper()
	wsID, chatID = s.Env.CreateWorkspaceWithChat(s.T(), s.imported.ProjectID, s.imported.RepoID, branch, parentID)
	return wsID, chatID, s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, wsID)
}

func (s *WorktreeSuite) getWorkspace(chatID string) map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), s.repoChatBase(chatID))
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var raw map[string]any
	kit.DecodeEnvData(s.T(), resp, &raw)
	return kit.WorktreeFrame(raw)
}

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

func (s *WorktreeSuite) workspaceChats() map[string]map[string]any {
	s.T().Helper()
	return s.Env.WorktreeChats(s.T(), s.imported.ProjectID, s.imported.RepoID)
}

func (s *WorktreeSuite) mergeIntoParent(childID, chatID, strategy string) string {
	s.T().Helper()
	before := s.getWorkspace(chatID)
	preMergeFork, _ := before["forkPointSha"].(string)

	watcher := s.Env.HoldProjects(s.T())
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
		"strategy": strategy,
	})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(s.T(), resp, 30*time.Second).State)
	merged := kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		fp, _ := m["forkPointSha"].(string)
		return fp != "" && fp != preMergeFork
	})
	parentTip, _ := merged["forkPointSha"].(string)
	return parentTip
}

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

func (s *WorktreeSuite) TestWorktree_mergeConflictsFalseForCleanChild() {
	t := s.T()
	_, chatID, worktreePath := s.createChild("feature/clean-merge")
	kit.CommitFile(t, worktreePath, "clean.txt", "clean\n", "clean commit")

	ws := s.getWorkspace(chatID)
	s.Assert().Equal(false, ws["mergeConflicts"], "a cleanly-mergeable child must report mergeConflicts:false")
	s.Assert().Equal(true, ws["canMergeLocally"], "structurally mergeable")
}

func (s *WorktreeSuite) TestWorktree_mergeDeleteSourceRemovesChild() {
	t := s.T()

	parentTipBefore := kit.RevParse(t, s.parentPath, "HEAD")

	childID, chatID, worktreePath := s.createChild("feature/merge-delete-source")
	kit.CommitFile(t, worktreePath, "md.txt", "child change\n", "child commit")

	watcher := s.Env.HoldProjects(t)
	resp := s.Env.POST(t, s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
		"strategy":     "merge",
		"deleteSource": true,
	})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(t, resp, 30*time.Second).State)

	watcher.Next(t, 10*time.Second, stream.KindChatEntity, func(m map[string]any) bool {
		return m["id"] == chatID && m["gone"] == true
	})

	s.Assert().NotEqual(parentTipBefore, kit.RevParse(t, s.parentPath, "HEAD"),
		"merge must advance the parent before the child is removed")
	s.Assert().False(kit.DirExists(t, worktreePath), "child worktree must be removed on disk")
	s.Assert().False(kit.BranchExists(t, s.imported.RepoPath, "feature/merge-delete-source"),
		"child branch must be force-deleted")
	_, stillListed := s.workspaceChats()[childID]
	s.Assert().False(stillListed, "deleted child must not appear in the workspace list")
}

func (s *WorktreeSuite) TestWorktree_mergeDeleteSourceKeepsNonLeafChild() {
	t := s.T()

	parentTipBefore := kit.RevParse(t, s.parentPath, "HEAD")

	childID, chatID, childPath := s.createChild("feature/non-leaf-parent")
	kit.CommitFile(t, childPath, "nl.txt", "child\n", "child commit")
	preMergeFork, _ := s.getWorkspace(chatID)["forkPointSha"].(string)
	grandchildID, _, _ := s.createChildUnder(childID, "feature/non-leaf-grandchild")

	watcher := s.Env.HoldProjects(t)
	resp := s.Env.POST(t, s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
		"strategy":     "merge",
		"deleteSource": true,
	})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(t, resp, 30*time.Second).State)

	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		fp, _ := m["forkPointSha"].(string)
		return fp != "" && fp != preMergeFork
	})
	s.Assert().NotEqual(parentTipBefore, kit.RevParse(t, s.parentPath, "HEAD"), "merge must advance the parent")

	chats := s.workspaceChats()
	s.Assert().Contains(chats, childID, "non-leaf merged child must be kept (deleting it would cascade the grandchild)")
	s.Assert().Contains(chats, grandchildID, "grandchild must survive")
}

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

func (s *WorktreeSuite) TestWorktree_reparent() {
	t := s.T()

	parentBID, _, parentBPath := s.createChild("feature/parent-b")
	kit.CommitFile(t, parentBPath, "pb.txt", "parent-b\n", "parent-b commit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	childID, childChatID, childPath := s.createChild("feature/child")
	kit.CommitFile(t, childPath, "child.txt", "child\n", "child commit")

	watcher := s.Env.HoldProjects(t)
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(t, resp, 30*time.Second).State)
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["forkPointSha"] == parentBTip
	})

	s.Assert().True(kit.FileExists(t, childPath+"/child.txt"), "child's own commit must survive reparent")
	s.Assert().True(kit.FileExists(t, childPath+"/pb.txt"), "new parent's history must be in child branch")

	reloaded := s.getWorkspace(childChatID)
	s.Assert().Equal(parentBID, reloaded["parentId"])
	s.Assert().Equal(parentBTip, reloaded["forkPointSha"])
}

func (s *WorktreeSuite) TestWorktree_reparentConflictMovesButStaysClean() {
	t := s.T()

	parentBID, _, parentBPath := s.createChild("feature/parent-b-conflict")
	kit.CommitFile(t, parentBPath, "shared.txt", "parent-b version\n", "parent-b edit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	childID, childChatID, childPath := s.createChild("feature/child-conflict")
	kit.CommitFile(t, childPath, "shared.txt", "child version\n", "child edit")

	watcher := s.Env.HoldProjects(t)
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{"newParentId": parentBID})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(t, resp, 30*time.Second).State)

	moved := kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["mergeConflicts"] == true
	})
	s.Assert().NotEqual(parentBTip, moved["forkPointSha"], "a conflicted reparent must not finalize onto the new tip")

	s.Assert().Equal("feature/child-conflict",
		kit.TrimNewline(kit.GitRun(t, childPath, "rev-parse", "--abbrev-ref", "HEAD")),
		"worktree must be on its branch, not a detached mid-rebase HEAD")
	s.Assert().Empty(kit.TrimNewline(kit.GitRun(t, childPath, "status", "--porcelain")),
		"worktree must be clean (no conflict markers, no in-progress rebase)")
	s.Assert().True(kit.FileExists(t, childPath+"/shared.txt"), "child's own work survives")
}

func (s *WorktreeSuite) TestWorktree_rebaseOntoParentConflictKeepsForResolve() {
	t := s.T()

	parentBID, _, parentBPath := s.createChild("feature/rop-parent")
	kit.CommitFile(t, parentBPath, "shared.txt", "parent version\n", "parent edit")
	parentBTip := kit.RevParse(t, parentBPath, "HEAD")

	childID, childChatID, childPath := s.createChild("feature/rop-child")
	kit.CommitFile(t, childPath, "shared.txt", "child version\n", "child edit")

	watcher := s.Env.HoldProjects(t)
	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{"newParentId": parentBID})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(t, resp, 30*time.Second).State)
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["mergeConflicts"] == true
	})

	resp2 := s.Env.POST(t, s.repoChatBase(childChatID)+"/rebase-onto-parent", map[string]any{})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(t, resp2, 30*time.Second).State)
	kit.WaitForWorkspace(t, watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["status"] == "pr-conflicts" && m["forkPointSha"] == parentBTip
	})

	got := s.getWorkspace(childChatID)
	s.Assert().Equal(parentBTip, got["forkPointSha"], "the intended fork point is persisted up front")
	s.Assert().Equal("HEAD",
		kit.TrimNewline(kit.GitRun(t, childPath, "rev-parse", "--abbrev-ref", "HEAD")),
		"the rebase is kept in progress (detached HEAD) for the user to resolve")
}

func (s *WorktreeSuite) TestWorktree_asyncOpSettlesItsOperation() {
	t := s.T()

	_, chatID, _ := s.createChild("feature/working-overlay")
	resp := s.Env.POST(t, s.repoChatBase(chatID)+"/sync", map[string]any{})
	op := s.Env.AwaitOperation(t, resp, 10*time.Second)
	s.Equal(domain.OpWorkspaceSync, op.Kind)
	s.Equal(domain.OpSucceeded, op.State)
}

func (s *WorktreeSuite) TestWorktree_reparentWithChildrenRejected() {
	t := s.T()

	parentBID, _, _ := s.createChild("feature/parent-b2")

	childID, childChatID, childPath := s.createChild("feature/child-rejected")
	kit.CommitFile(t, childPath, "c.txt", "c\n", "child commit")
	childTipBefore := kit.RevParse(t, childPath, "HEAD")

	s.createChildUnder(childID, "feature/grandchild-rejected")

	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{
		"newParentId": parentBID,
	})
	s.Equal(domain.OpFailed, s.Env.AwaitOperation(t, resp, 5*time.Second).State)

	childTipAfterRejection := kit.RevParse(t, childPath, "HEAD")
	s.Assert().Equal(childTipBefore, childTipAfterRejection, "rejected reparent must not mutate git")

	reloaded := s.getWorkspace(childChatID)
	s.Assert().Equal(s.parentID, reloaded["parentId"], "rejected reparent must keep the original parent")
}

func (s *WorktreeSuite) TestRegression_ReparentOntoSelfRejected() {
	t := s.T()

	childID, childChatID, childPath := s.createChild("feature/self-parent")
	kit.CommitFile(t, childPath, "c.txt", "c\n", "child commit")
	childTipBefore := kit.RevParse(t, childPath, "HEAD")

	resp := s.Env.POST(t, s.repoChatBase(childChatID)+"/reparent", map[string]any{
		"newParentId": childID,
	})
	op := s.Env.AwaitOperation(t, resp, 5*time.Second)
	s.Require().Equal(domain.OpFailed, op.State)
	s.Require().NotNil(op.Error)
	s.Contains(op.Error.Message, "itself", "self-parent must be rejected with a clear error")

	reloaded := s.getWorkspace(childChatID)
	s.Assert().Equal(s.parentID, reloaded["parentId"], "self-parent must not change the parent")
	s.Assert().NotEqual(childID, reloaded["parentId"], "workspace must never become its own parent")
	s.Assert().Equal(childTipBefore, kit.RevParse(t, childPath, "HEAD"), "rejected self-parent must not mutate git")
}

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

func (s *WorktreeSuite) TestRegression_MergeEligibility_SiblingState() {
	t := s.T()

	parentID, _, _ := s.createChild("feature/elig-parent")

	_, childChatID := s.Env.CreateWorkspaceWithChat(t, s.imported.ProjectID, s.imported.RepoID, "feature/elig-child", parentID)
	created := s.getWorkspace(childChatID)
	can, _ := created["canMergeLocally"].(bool)
	s.Assert().True(can && created["parentBranch"] == "feature/elig-parent",
		"an idle parent must make a freshly created child eligible to merge locally")

	parentWatcher := s.Env.HoldProjects(t)
	s.Env.PushProviderState(t, parentID, kit.ProviderState{Protected: true})
	kit.WaitForWorkspaceState(t, parentWatcher, parentID, "locked", 5*time.Second)

	child := s.getWorkspace(childChatID)
	can2, _ := child["canMergeLocally"].(bool)
	s.Assert().False(can2, "a locked parent must make the child ineligible to merge locally")
}

func (s *WorktreeSuite) TestWorktree_deleteCascadeSkipsLockedChild() {
	t := s.T()

	rootID, rootChatID, _ := s.createChild("feature/root-cascade")
	lockedID, _, _ := s.createChildUnder(rootID, "feature/locked-branch")

	lockWatcher := s.Env.HoldProjects(t)
	s.Env.PushProviderState(t, lockedID, kit.ProviderState{Protected: true})
	kit.WaitForWorkspaceState(t, lockWatcher, lockedID, "locked", 5*time.Second)

	rootWatcher := s.Env.HoldProjects(t)
	resp := s.Env.DELETE(t, s.repoChatBase(rootChatID))
	kit.RequireStatus(t, resp, http.StatusOK)
	var deleted map[string]any
	kit.DecodeEnvData(t, resp, &deleted)
	s.Require().Empty(deleted["takenChatIds"], "the locked child's chat is not taken with the root")
	rootWatcher.Next(t, 5*time.Second, stream.KindChatEntity, func(m map[string]any) bool {
		return m["id"] == rootChatID && m["gone"] == true
	})

	s.Env.QuiesceReactors()

	chats := s.workspaceChats()
	s.Require().Contains(chats, lockedID, "cascade delete must KEEP the locked child")
	s.Require().NotContains(chats, rootID, "cascade delete must reap the unlocked root")
}
