//go:build integration

package conflicts_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

type ConflictsSuite struct {
	kit.IntegrationSuite
	imported     kit.ImportedRepo
	parentID     string
	parentChatID string
	parentPath   string
}

func (s *ConflictsSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "conflicts", "")
	s.parentID, s.parentChatID = s.Env.CreateWorkspaceWithChat(
		s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/conflicts-base", "",
	)
	s.parentPath = s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, s.parentID)
}

func (s *ConflictsSuite) repoChatBase(chatID string) string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/chats/" + chatID
}

func (s *ConflictsSuite) chatBase(chatID string) string {
	return "/v0/chats/" + chatID
}

func (s *ConflictsSuite) createChildWithChat(parentID, branch string) (wsID, chatID, path string) {
	s.T().Helper()
	wsID, chatID = s.Env.CreateWorkspaceWithChat(s.T(), s.imported.ProjectID, s.imported.RepoID, branch, parentID)
	return wsID, chatID, s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, wsID)
}

func (s *ConflictsSuite) getWorkspace(chatID string) map[string]any {
	s.T().Helper()
	resp := s.Env.GET(s.T(), s.repoChatBase(chatID))
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	var raw map[string]any
	kit.DecodeEnvData(s.T(), resp, &raw)
	return kit.WorktreeFrame(raw)
}

func TestConflictsSuite(t *testing.T) {
	suite.Run(
		t,
		new(ConflictsSuite),
	)
}

func (s *ConflictsSuite) conflictSetup() (childID, chatID string) {
	s.T().Helper()

	kit.CommitFile(
		s.T(),
		s.parentPath,
		"shared.txt",
		"base line\n",
		"base",
	)

	childID, chatID, childWorktreePath := s.createChildWithChat(s.parentID, "feature/conflict")

	kit.CommitFile(
		s.T(),
		childWorktreePath,
		"shared.txt",
		"child version\n",
		"child edit",
	)
	kit.CommitFile(
		s.T(),
		s.parentPath,
		"shared.txt",
		"parent version\n",
		"parent edit",
	)
	return childID, chatID
}

func (s *ConflictsSuite) mergeConflict(childID, chatID string) {
	s.T().Helper()
	childPath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)

	watcher := s.Env.HoldProjects(s.T())
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/merge-into-parent", map[string]string{
		"strategy": "merge",
	})

	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(s.T(), resp, 30*time.Second).State)
	kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["status"] == "pr-conflicts"
	})
	s.Env.Quiesce()

	s.requireClean(childPath, "child")
	s.requireClean(s.parentPath, "parent")
}

func (s *ConflictsSuite) requireClean(worktreePath, label string) {
	s.T().Helper()
	porcelain := kit.TrimNewline(kit.GitRun(s.T(), worktreePath, "status", "--porcelain"))
	head := kit.TrimNewline(kit.GitRun(s.T(), worktreePath, "rev-parse", "--abbrev-ref", "HEAD"))
	s.Require().Empty(porcelain, "%s worktree must be clean (no conflict markers left behind)", label)
	s.Require().NotEqual("HEAD", head, "%s worktree must not be stuck on a detached mid-rebase HEAD", label)
}

func (s *ConflictsSuite) keptRebaseConflictOnChild() (childID, chatID string) {
	s.T().Helper()

	parentBID, _, parentBPath := s.createChildWithChat(s.parentID, "feature/conflict-parent-b")
	kit.CommitFile(s.T(), parentBPath, "shared.txt", "parent-b version\n", "parent-b edit")
	parentBTip := kit.RevParse(s.T(), parentBPath, "HEAD")

	childID, chatID, childPath := s.createChildWithChat(s.parentID, "feature/conflict-rebase-child")
	kit.CommitFile(s.T(), childPath, "shared.txt", "child version\n", "child edit")

	watcher := s.Env.HoldProjects(s.T())
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/reparent", map[string]any{"newParentId": parentBID})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(s.T(), resp, 30*time.Second).State)
	kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["parentId"] == parentBID && m["mergeConflicts"] == true
	})

	resp2 := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/rebase-onto-parent", map[string]any{})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(s.T(), resp2, 30*time.Second).State)
	kit.WaitForWorkspace(s.T(), watcher, childID, 10*time.Second, func(m map[string]any) bool {
		return m["status"] == "pr-conflicts" && m["forkPointSha"] == parentBTip
	})
	return childID, chatID
}

func (s *ConflictsSuite) TestConflicts_mergeDetectsConflict() {
	childID, chatID := s.conflictSetup()
	s.mergeConflict(childID, chatID)

	reloaded := s.getWorkspace(chatID)
	s.Assert().Equal("pr-conflicts", reloaded["status"],
		"child workspace must be pr-conflicts after a conflicting merge")

	childPath := s.Env.WorktreePath(s.imported.ProjectID, s.imported.RepoID, childID)
	s.Assert().Empty(kit.TrimNewline(kit.GitRun(s.T(), s.parentPath, "status", "--porcelain")),
		"parent worktree must be clean after a conflicting merge (never bricked)")
	s.Assert().Empty(kit.TrimNewline(kit.GitRun(s.T(), childPath, "status", "--porcelain")),
		"child worktree must be clean after a conflicting merge")
}

func (s *ConflictsSuite) TestConflicts_mergeConflictsPredictedBeforeMerge() {
	_, chatID := s.conflictSetup()

	ws := s.getWorkspace(chatID)

	s.Assert().Equal(true, ws["mergeConflicts"],
		"a child that would conflict must report mergeConflicts:true before any merge")
	s.Assert().Equal(true, ws["canMergeLocally"],
		"canMergeLocally stays structurally true")
}

func (s *ConflictsSuite) TestConflicts_mergeConflictsDeliveredOnBroadcast() {
	childID, chatID := s.conflictSetup()

	watcher := s.Env.HoldProjects(s.T())
	resp := s.Env.PATCH(s.T(), s.chatBase(chatID)+"/review", map[string]any{"mergeStrategy": "squash"})
	kit.RequireStatus(s.T(), resp, http.StatusOK)
	resp.Body.Close()

	got := kit.WaitForWorkspace(s.T(), watcher, childID, 5*time.Second, func(m map[string]any) bool {
		return m["mergeStrategy"] == "squash"
	})
	s.Assert().Equal(true, got["mergeConflicts"],
		"the live broadcast must carry the predicted merge conflict, not only the snapshot read")
}

func (s *ConflictsSuite) TestConflicts_mergeDeleteSourceKeepsConflictedChild() {
	childID, chatID := s.conflictSetup()

	watcher := s.Env.HoldProjects(s.T())
	resp := s.Env.POST(s.T(), s.repoChatBase(chatID)+"/merge-into-parent", map[string]any{
		"strategy":     "merge",
		"deleteSource": true,
	})
	s.Require().Equal(domain.OpSucceeded, s.Env.AwaitOperation(s.T(), resp, 30*time.Second).State)
	kit.WaitForWorkspaceState(s.T(), watcher, childID, "pr-conflicts", 5*time.Second)

	getResp := s.Env.GET(s.T(), s.repoChatBase(chatID))
	kit.RequireStatus(s.T(), getResp, http.StatusOK)
}

func (s *ConflictsSuite) TestConflicts_conflictedFilesListsFile() {
	_, chatID := s.keptRebaseConflictOnChild()

	conflictsResp := s.Env.GET(s.T(), s.chatBase(chatID)+"/git/conflicts")
	kit.RequireStatus(s.T(), conflictsResp, http.StatusOK)

	var files []string
	kit.DecodeEnvData(s.T(), conflictsResp, &files)
	s.Require().NotEmpty(files)
	s.Assert().Contains(files, "shared.txt")
}

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

func (s *ConflictsSuite) TestRegression_MergeConflictLeavesCleanParentResolvableViaRebase() {
	childID, chatID := s.conflictSetup()
	s.mergeConflict(childID, chatID)

	statusResp := s.Env.GET(s.T(), s.chatBase(s.parentChatID)+"/git/status")
	kit.RequireStatus(s.T(), statusResp, http.StatusOK)
	var status map[string]any
	kit.DecodeEnvData(s.T(), statusResp, &status)
	files, _ := status["files"].([]any)
	s.Assert().Empty(files, "parent must already be clean after a conflicting merge (never stuck)")

	abortResp := s.Env.POST(s.T(), s.chatBase(s.parentChatID)+"/git/operation/abort", nil)
	abortResp.Body.Close()
	s.Assert().NotEqual(http.StatusOK, abortResp.StatusCode,
		"no in-progress op should remain on the parent to abort")

	reloaded := s.getWorkspace(chatID)
	s.Assert().Equal("pr-conflicts", reloaded["status"],
		"the conflict surfaces as the child's pr-conflicts state")
}
