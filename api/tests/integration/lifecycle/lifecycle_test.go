//go:build integration

package lifecycle_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test entry point; delegates to kit.Main.
func TestMain(m *testing.M) {
	kit.Main(m)
}

// LifecycleSuite verifies end-to-end lifecycle flows through the full HTTP+WS+SQLite stack.
type LifecycleSuite struct {
	kit.IntegrationSuite
}

// TestLifecycleSuite runs the LifecycleSuite via testify.
func TestLifecycleSuite(t *testing.T) {
	suite.Run(
		t,
		new(LifecycleSuite),
	)
}

// TestLifecycle_WorkspaceCreateBroadcastsOnWS proves the projection → stream
// chain: a worktree's state reaches a projects-scope holder as its workspace
// row, carrying its branch and repo. It creates through the usecase
// (kit.Env.CreateWorkspaceWithChat) because no headless HTTP create names a
// branch, and keys on the worktree's own sync, a write made after the dial.
func (s *LifecycleSuite) TestLifecycle_WorkspaceCreateBroadcastsOnWS() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "lifecycle", "")
	wsID, chatID := s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/lifecycle", "")

	watcher := s.Env.HoldProjects(t)
	syncResp := s.Env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+chatID+"/sync", nil)
	kit.RequireStatus(t, syncResp, http.StatusAccepted)
	syncResp.Body.Close()

	created := kit.WaitForWorkspace(t, watcher, wsID, 5*time.Second, func(m map[string]any) bool {
		return m["branch"] == "feature/lifecycle"
	})
	s.Assert().Equal(imported.RepoID, created["repoId"])
}

// TestLifecycle_SyncSucceedsAfterACommit proves a workspace edit → stage →
// commit → sync flow round-trips: the sync operation succeeds and the workspace
// stays status "new" (per D4 — status is not cleared by HasCommits).
func (s *LifecycleSuite) TestLifecycle_SyncSucceedsAfterACommit() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "lifecycle-sync", "")
	wsID, chatID := s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/lifecycle-sync", "")
	chatBase := "/v0/chats/" + chatID

	saveResp := s.Env.PUT(t, "/v0/documents/content", map[string]any{
		"workspaceId": wsID,
		"path":        "a.txt",
		"content":     "aaa\n",
	})
	kit.RequireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	stageResp := s.Env.POST(t, chatBase+"/git/stage", map[string]any{
		"paths": []string{"a.txt"},
	})
	kit.RequireStatus(t, stageResp, http.StatusOK)
	stageResp.Body.Close()

	commitResp := s.Env.POST(t, chatBase+"/git/commit", map[string]any{
		"subject": "add sync test file",
		"author":  "Test <t@t.com>",
	})
	kit.RequireStatus(t, commitResp, http.StatusOK)
	commitResp.Body.Close()

	watcher := s.Env.HoldProjects(t)
	syncResp := s.Env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+chatID+"/sync", nil)
	s.Equal(domain.OpSucceeded, s.Env.AwaitOperation(t, syncResp, 10*time.Second).State)
	kit.WaitForWorkspaceState(t, watcher, wsID, "new", 5*time.Second)
}

// TestLifecycle_MergeEligibilityTrueWhenParentIdle proves that a child whose
// parent is idle (not locked/deleted) carries canMergeLocally:true and the
// parent's branch in its workspace row (spec §10), read back over REST.
func (s *LifecycleSuite) TestLifecycle_MergeEligibilityTrueWhenParentIdle() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "merge-elig", "")
	parentID := s.Env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/parent")

	_, childChatID := s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/child", parentID)
	resp := s.Env.GET(t, "/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+childChatID)
	kit.RequireStatus(t, resp, http.StatusOK)
	var raw map[string]any
	kit.DecodeEnvData(t, resp, &raw)
	msg := kit.WorktreeFrame(raw)

	can, _ := msg["canMergeLocally"].(bool)
	s.Assert().True(can, "an idle parent must make the child eligible to merge locally")
	s.Assert().Equal("feature/parent", msg["parentBranch"])
	s.Assert().Equal(parentID, msg["parentId"])
}

// TestLifecycle_HealthEndpoint verifies the health route responds 200.
func (s *LifecycleSuite) TestLifecycle_HealthEndpoint() {
	t := s.T()

	resp := s.Env.GET(t, "/v0/health")
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

// TestLifecycle_WorkspaceList verifies create then list round-trips over the
// repo-scoped chat list — the read-model replacement for the deleted workspace
// list (spec §8 step 6; kit.Env.WorktreeChats).
//
// It creates with CreateWorkspaceWithChat, not the bare CreateWorkspace: the
// chat list is keyed by OWNING CHAT (each worktree-owning row's own id), and a
// bare create mints no chat at all — it is owed one only by the next boot's
// backfill (kit.Env.OwningChatID's own doc) — so an un-chatted workspace would
// never appear in this list no matter how long Quiesce waits.
func (s *LifecycleSuite) TestLifecycle_WorkspaceList() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "ws-list", "")

	ids := make([]string, 3)
	for i, branch := range []string{"feature/a", "feature/b", "feature/c"} {
		ids[i], _ = s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	}

	// CreateWorkspaceWithChat returns once its writes are observed on the hub
	// broadcast (WS), but the list route reads the store projection — an
	// independent async read model that can trail the WS frame. Drain asynx so
	// both projections are settled, then assert the snapshot deterministically
	// (no polling, no timeout).
	s.Env.Quiesce()

	chats := s.Env.WorktreeChats(t, imported.ProjectID, imported.RepoID)
	for _, wsID := range ids {
		s.Assert().Contains(chats, wsID, "workspace %q must appear in the chat list", wsID)
	}
}

// TestLifecycle_GitStageCommitChangesStatus proves file edit → stage → commit
// makes the working tree clean again (git status-driven via HTTP) on a writable
// child workspace.
func (s *LifecycleSuite) TestLifecycle_GitStageCommitChangesStatus() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "git-status", "")
	wsID, chatID := s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/lifecycle-git", "")
	chatBase := "/v0/chats/" + chatID

	saveResp := s.Env.PUT(t, "/v0/documents/content", map[string]any{
		"workspaceId": wsID,
		"path":        "hello.txt",
		"content":     "Hello, world!\n",
	})
	kit.RequireStatus(t, saveResp, http.StatusOK)
	saveResp.Body.Close()

	statusResp := s.Env.GET(t, chatBase+"/git/status")
	kit.RequireStatus(t, statusResp, http.StatusOK)

	var statusObj map[string]any
	kit.DecodeEnvData(t, statusResp, &statusObj)

	files, _ := statusObj["files"].([]any)
	s.Assert().NotEmpty(files, "edited file must appear in status")

	stageResp := s.Env.POST(t, chatBase+"/git/stage", map[string]any{
		"paths": []string{"hello.txt"},
	})
	kit.RequireStatus(t, stageResp, http.StatusOK)
	stageResp.Body.Close()

	commitResp := s.Env.POST(t, chatBase+"/git/commit", map[string]any{
		"subject": "Add hello.txt",
		"author":  "Test <t@t.com>",
	})
	kit.RequireStatus(t, commitResp, http.StatusOK)
	commitResp.Body.Close()

	statusResp2 := s.Env.GET(t, chatBase+"/git/status")
	kit.RequireStatus(t, statusResp2, http.StatusOK)

	var statusObj2 map[string]any
	kit.DecodeEnvData(t, statusResp2, &statusObj2)

	files2, _ := statusObj2["files"].([]any)
	s.Assert().Empty(files2, "working tree must be clean after commit")
}
