//go:build integration

package websocket_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// WebSocketSuite exercises the projects stream scope end-to-end.
type WebSocketSuite struct {
	suite.Suite
	env *kit.Env
}

func (s *WebSocketSuite) SetupTest() {
	s.env = kit.BuildEnv(s.T())
}

func TestWebSocketSuite(t *testing.T) {
	suite.Run(t, new(WebSocketSuite))
}

// triggerWorktreeEvent pushes a mock-provider poll result for wsID: a real,
// deterministic workspace mutation whose row reaches the projects scope live.
func triggerWorktreeEvent(t *testing.T, env *kit.Env, wsID, prURL string) {
	t.Helper()
	env.PushProviderState(t, wsID, kit.ProviderState{
		HasPR:    true,
		PRStatus: "open",
		PRUrl:    prURL,
		PRTitle:  "t",
	})
}

// prURLIs matches a workspace row carrying url.
func prURLIs(url string) func(map[string]any) bool {
	return func(m map[string]any) bool { return m["prUrl"] == url }
}

// TestWS_ProjectsScopeCarriesAWorkspaceChange proves a workspace's live change
// reaches a projects-scope holder as the row its snapshot would carry.
func (s *WebSocketSuite) TestWS_ProjectsScopeCarriesAWorkspaceChange() {
	t := s.T()

	imported := s.env.ImportRepo(t, "repo-scope", "")
	wsID, _ := s.env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/x", "")

	watcher := s.env.WatchProjects(t)
	triggerWorktreeEvent(t, s.env, wsID, "https://example.test/pr/1")

	msg := kit.WaitForWorkspace(t, watcher, wsID, 5*time.Second, prURLIs("https://example.test/pr/1"))
	s.Assert().Equal(imported.RepoID, msg["repoId"])
	s.Assert().Equal(imported.ProjectID, msg["projectId"])
}

// TestWS_ProjectsScopeCarriesAnImportedRepo proves an imported repo reaches a
// projects-scope holder under its project.
func (s *WebSocketSuite) TestWS_ProjectsScopeCarriesAnImportedRepo() {
	t := s.T()

	repoPath := kit.InitRepo(t)
	projectID := s.env.RegisterProject(t, "proj-scope", repoPath)

	watcher := s.env.WatchProjects(t)
	resp := s.env.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": "repo-scope",
		"path": repoPath,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// The importer derives the repo name from the on-disk directory, so match on
	// its project, not the request body name.
	msg := watcher.Next(t, 5*time.Second, stream.KindRepo, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		return m["projectId"] == projectID && id != ""
	})
	s.Assert().Equal(projectID, msg["projectId"])
}

// TestWS_MultiClientFanOut verifies that a workspace change reaches every
// connection holding the projects scope.
func (s *WebSocketSuite) TestWS_MultiClientFanOut() {
	t := s.T()

	imported := s.env.ImportRepo(t, "fan-out", "")
	wsID, _ := s.env.CreateWorkspaceWithChat(t, imported.ProjectID, imported.RepoID, "feature/fan", "")

	watcher1 := s.env.WatchProjects(t)
	watcher2 := s.env.WatchProjects(t)

	triggerWorktreeEvent(t, s.env, wsID, "https://example.test/pr/fan")

	msg1 := kit.WaitForWorkspace(t, watcher1, wsID, 5*time.Second, prURLIs("https://example.test/pr/fan"))
	msg2 := kit.WaitForWorkspace(t, watcher2, wsID, 5*time.Second, prURLIs("https://example.test/pr/fan"))
	s.Assert().Equal(wsID, msg1["id"])
	s.Assert().Equal(wsID, msg2["id"])
}
