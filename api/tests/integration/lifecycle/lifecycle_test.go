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

func TestMain(m *testing.M) {
	kit.Main(m)
}

type LifecycleSuite struct {
	kit.IntegrationSuite
}

func TestLifecycleSuite(t *testing.T) {
	suite.Run(
		t,
		new(LifecycleSuite),
	)
}

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

func (s *LifecycleSuite) TestLifecycle_HealthEndpoint() {
	t := s.T()

	resp := s.Env.GET(t, "/v0/health")
	defer resp.Body.Close()
	s.Require().Equal(http.StatusOK, resp.StatusCode)
}

func (s *LifecycleSuite) TestLifecycle_WorkspaceList() {
	t := s.T()

	imported := s.Env.ImportRepo(t, "ws-list", "")

	ids := make([]string, 3)
	for i, branch := range []string{"feature/a", "feature/b", "feature/c"} {
		ids[i], _ = s.Env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, branch, "")
	}

	s.Env.Quiesce()

	chats := s.Env.WorktreeChats(t, imported.ProjectID, imported.RepoID)
	for _, wsID := range ids {
		s.Assert().Contains(chats, wsID, "workspace %q must appear in the chat list", wsID)
	}
}

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
