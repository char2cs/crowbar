//go:build integration

package lifecycle_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

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

// TestLifecycle_WorkspaceCreateBroadcastsOnWS proves the projection → hub
// broadcast → WS chain still holds under the chat-scoped surface (spec §4/§5):
// a worktree's state is observable on the repo's chat feed as a worktree_state
// frame, carrying its branch and repo.
//
// It creates through the USECASE (kit.Env.CreateWorkspaceWithChat), not HTTP —
// spec §8 step 6 deleted POST .../workspaces, the only HTTP surface that could
// name a branch, and the one HTTP create left (POST .../chats with
// ownWorktree:true) both auto-derives the branch name AND synchronously spawns
// a real provider CLI runner (chat/internal/tree/chats.go
// createOwnWorktreeChat) — this suite's kit.Env registers no stub provider, so
// that path is not exercisable headless. See kit.Env.CreateWorkspace's own doc.
//
// It also can no longer key on the CREATE's own broadcast: pushChatWorktree
// pushes NOTHING for a workspace with no resolved owning chat (container.go),
// and a bare create mints none — the boot backfill (or, here,
// CreateWorkspaceWithChat's own OwningChatID call) is what gives it one, AFTER
// the create has already committed and been dropped by the push. So this pins
// the first broadcast a freshly-chatted worktree CAN produce — its own sync —
// rather than the create event itself.
func (s *LifecycleSuite) TestLifecycle_WorkspaceCreateBroadcastsOnWS() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "lifecycle", "")
	wsID, chatID := s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/lifecycle", "")

	watcher := s.Env.DialRepoChats(t, imported.ProjectID, imported.RepoID)
	syncResp := s.Env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+chatID+"/sync", nil)
	kit.RequireStatus(t, syncResp, http.StatusAccepted)
	syncResp.Body.Close()

	created := kit.WaitForWorkspace(t, watcher, wsID, 5*time.Second, func(m map[string]any) bool {
		return m["branch"] == "feature/lifecycle"
	})
	s.Assert().Equal(imported.RepoID, created["repoId"])
}

// TestLifecycle_SyncClearsLastError proves a workspace edit → stage → commit →
// sync flow round-trips: the workspace stays status "new" after committing (per
// D4 — status is not cleared by HasCommits) and carries no lastError.
func (s *LifecycleSuite) TestLifecycle_SyncClearsLastError() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "lifecycle-sync", "")
	wsID, chatID := s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/lifecycle-sync", "")
	chatBase := "/v0/chats/" + chatID

	saveResp := s.Env.PUT(t, chatBase+"/files/content", map[string]any{
		"path":    "a.txt",
		"content": "aaa\n",
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

	watcher := s.Env.DialChat(t, chatID)
	syncResp := s.Env.POST(t,
		"/v0/projects/"+imported.ProjectID+"/repos/"+imported.RepoID+"/chats/"+chatID+"/sync", nil)
	kit.RequireStatus(t, syncResp, http.StatusAccepted)
	syncResp.Body.Close()

	// Per D4 status STAYS "new" after sync with commits; the workspace must carry
	// no lastError. Wait on the terminal "new" frame and assert lastError empty.
	msg := kit.WaitForWorkspaceState(t, watcher, wsID, "new", 5*time.Second)
	le, _ := msg["lastError"].(string)
	s.Assert().Empty(le, "sync must not set a lastError")
}

// TestLifecycle_MergeEligibilityTrueWhenParentIdle proves that a child whose
// parent is idle (not locked/deleted) carries canMergeLocally:true and the
// parent's branch in the WS DTO (spec §10).
//
// It reads the freshly created child over REST rather than waiting on a WS
// frame: a bare create mints no owning chat, and pushChatWorktree pushes
// NOTHING without one (container.go) — so the create's own broadcast is
// unobservable by construction, and CreateWorkspaceWithChat's own chat-mint
// does not retroactively resend it (mintOwningChat writes the CHAT aggregate,
// never re-committing the workspace).
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
	_, chatID := s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/lifecycle-git", "")
	chatBase := "/v0/chats/" + chatID

	saveResp := s.Env.PUT(t, chatBase+"/files/content", map[string]any{
		"path":    "hello.txt",
		"content": "Hello, world!\n",
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
