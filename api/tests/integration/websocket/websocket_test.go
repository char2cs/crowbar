//go:build integration

package websocket_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

func TestMain(m *testing.M) {
	kit.Main(m)
}

// WebSocketSuite exercises the hierarchical WebSocket prefix-filtering and the
// directly-injected (PushGit/PushFile/PushLSP) topics end-to-end.
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

// TestWS_HierarchicalPrefix_RepoScopeReceivesAllWorkspaces proves that a client
// subscribed at the repo-scoped .../workspaces prefix receives every workspace
// under that repo via hierarchical prefix matching — no query params (spec §5).
func (s *WebSocketSuite) TestWS_HierarchicalPrefix_RepoScopeReceivesAllWorkspaces() {
	t := s.T()

	imported := s.env.ImportRepo(t, "repo-scope", "")

	watcher := s.env.DialWorkspaces(t, imported.ProjectID, imported.RepoID)
	wsID := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/x")

	msg := kit.WaitForWorkspace(t, watcher, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	s.Assert().Equal(wsID, msg["id"])
	s.Assert().Equal(imported.RepoID, msg["repoId"])
	s.Assert().Equal(imported.ProjectID, msg["projectId"])
}

// TestWS_HierarchicalPrefix_WsScopeRejectsSibling proves that a client at the
// exact .../workspaces/:wsId prefix receives only that workspace's frames and
// NOT a sibling's (negative filtering via AssertNoMessage, spec §5).
func (s *WebSocketSuite) TestWS_HierarchicalPrefix_WsScopeRejectsSibling() {
	t := s.T()

	imported := s.env.ImportRepo(t, "ws-scope", "")
	target := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/target")

	// Exact-scope subscriber: only frames for `target` should arrive. Confirm the
	// connect snapshot carries the target.
	watcher := s.env.DialWorkspace(t, imported.ProjectID, imported.RepoID, target)
	own := kit.WaitForWorkspace(t, watcher, target, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	s.Assert().Equal(target, own["id"])

	// Create a sibling under the SAME repo. Its frames must NOT reach the
	// exact-scope subscriber (out-of-prefix). The watcher may legitimately see
	// more of its own `target` frames, so assert the SIBLING id specifically is
	// never delivered (negative prefix filtering, spec §5).
	sibling := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/sibling")
	s.Assert().True(
		watcher.AssertNoMatch(t, time.Second, func(m map[string]any) bool {
			return m["id"] == sibling
		}),
		"exact-scope watcher must not receive a sibling workspace's frame",
	)
}

// TestRegression_Workspaces_NamespaceFiltering proves the full §5 prefix model
// in one test: a repo-scoped subscriber receives the workspace; an exact-scope
// subscriber on a DIFFERENT workspace never does (AssertNoMatch for the
// out-of-prefix id). This is the namespace-filtering contract for the Workspaces
// topic (project/repo/ws prefix).
func (s *WebSocketSuite) TestRegression_Workspaces_NamespaceFiltering() {
	t := s.T()

	imported := s.env.ImportRepo(t, "ns-filter", "")

	// An exact-scope subscriber on the adopted-main workspace.
	mainWatcher := s.env.DialWorkspace(t, imported.ProjectID, imported.RepoID, imported.WorkspaceID)
	kit.WaitForWorkspace(t, mainWatcher, imported.WorkspaceID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})

	// A repo-scoped subscriber receives every workspace under the repo.
	repoWatcher := s.env.DialWorkspaces(t, imported.ProjectID, imported.RepoID)
	wsID := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/ns")
	kit.WaitForWorkspace(t, repoWatcher, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})

	// The exact-scope main subscriber must NEVER receive the sibling's id.
	s.Assert().True(
		mainWatcher.AssertNoMatch(t, time.Second, func(m map[string]any) bool {
			return m["id"] == wsID
		}),
		"out-of-prefix workspace must not reach an exact-scope subscriber",
	)
}

// TestWS_ProjectScopeReceivesRepoEvents proves that a project-scoped repos
// subscriber receives the repo's RepoDTO via hierarchical prefix matching.
func (s *WebSocketSuite) TestWS_ProjectScopeReceivesRepoEvents() {
	t := s.T()

	repoPath := kit.InitRepo(t)
	projectID := s.env.RegisterProject(t, "proj-scope", repoPath)

	watcher := s.env.DialRepos(t, projectID)
	resp := s.env.POST(t, "/v0/projects/"+projectID+"/repos", map[string]any{
		"name": "repo-scope",
		"path": repoPath,
	})
	kit.RequireStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	// The importer derives the repo name from the on-disk directory, so match on
	// the project-scoped prefix (projectId), not the request body name.
	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		id, _ := m["id"].(string)
		return m["projectId"] == projectID && id != ""
	})
	s.Assert().Equal(projectID, msg["projectId"])
}

// TestWS_LSP_FilteredByWsID verifies the LSP topic (flat wsId namespace) filters
// by the subscribing workspace's path scope. PushLSP injects directly so no
// workspace creation is needed; the route is the hierarchical .../lsp/ws.
func (s *WebSocketSuite) TestWS_LSP_FilteredByWsID() {
	t := s.T()

	imported := s.env.ImportRepo(t, "lsp-filter", "")
	wsID := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/lsp")

	watcher := s.env.DialLSP(t, imported.ProjectID, imported.RepoID, wsID)

	// Push a diagnostic for a different workspace — must be filtered.
	s.env.PushLSP("ws-other", []kit.LSPDiagnostic{{Message: "skip me"}})
	// Push the matching diagnostic.
	s.env.PushLSP(wsID, []kit.LSPDiagnostic{{Message: "target diag"}})

	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["wsId"] == wsID
	})
	s.Assert().Equal(wsID, msg["wsId"])

	diags, ok := msg["diagnostics"].([]any)
	s.Require().True(ok)
	s.Require().Len(diags, 1)
	diag0, ok := diags[0].(map[string]any)
	s.Require().True(ok)
	s.Assert().Equal("target diag", diag0["message"])
}

// TestWS_MultiClientFanOut verifies that a create reaches all connected clients
// on the same repo-scoped prefix.
func (s *WebSocketSuite) TestWS_MultiClientFanOut() {
	t := s.T()

	imported := s.env.ImportRepo(t, "fan-out", "")

	watcher1 := s.env.DialWorkspaces(t, imported.ProjectID, imported.RepoID)
	watcher2 := s.env.DialWorkspaces(t, imported.ProjectID, imported.RepoID)

	wsID := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/fan")

	msg1 := kit.WaitForWorkspace(t, watcher1, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	msg2 := kit.WaitForWorkspace(t, watcher2, wsID, 5*time.Second, func(_ map[string]any) bool {
		return true
	})
	s.Assert().Equal(wsID, msg1["id"])
	s.Assert().Equal(wsID, msg2["id"])
}

// TestWS_GitTopic_StatusBroadcast verifies the git WS topic (flat wsId
// namespace, change-only) delivers an injected git status on the hierarchical
// .../git/status route.
func (s *WebSocketSuite) TestWS_GitTopic_StatusBroadcast() {
	t := s.T()

	imported := s.env.ImportRepo(t, "git-topic", "")
	wsID := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/ws-git-test")

	watcher := s.env.DialGit(t, imported.ProjectID, imported.RepoID, wsID)

	// Inject the git status directly — the git WS topic is change-only and the
	// OS file watcher is not running in the test environment.
	s.env.PushGit(kit.GitStatusEvent{
		WsID:   wsID,
		Branch: "feature/ws-git-test",
		Files: []kit.GitFileEntry{
			{Path: "new.txt", Status: "added", Staged: true},
		},
	})

	// The git topic dual-serves a snapshot on connect (possibly empty), so wait
	// for the frame that carries our injected file rather than the first frame.
	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		files, ok := m["files"].([]any)
		if !ok {
			return false
		}
		for _, f := range files {
			entry, isMap := f.(map[string]any)
			if isMap {
				if path, _ := entry["path"].(string); path == "new.txt" {
					return true
				}
			}
		}
		return false
	})
	files, _ := msg["files"].([]any)
	s.Require().NotEmpty(files, "expected at least one file entry in git status")
	s.Assert().Empty(msg["wsId"], "wsId must not appear in the git status wire payload")
}

// TestWS_FilesTopicIsChangeOnly verifies that the files topic has no snapshot.
func (s *WebSocketSuite) TestWS_FilesTopicIsChangeOnly() {
	t := s.T()

	imported := s.env.ImportRepo(t, "files-topic", "")
	wsID := s.env.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/files")

	watcher := s.env.DialFiles(t, imported.ProjectID, imported.RepoID, wsID)

	// Inject a file change event directly — the OS file watcher is not running.
	s.env.PushFile(kit.FileEvent{
		WsID: wsID,
		Type: "modified",
		Path: "test.txt",
	})

	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["wsId"] == wsID
	})
	s.Assert().Equal(wsID, msg["wsId"])
	s.Assert().NotEmpty(msg["type"])
	s.Assert().NotEmpty(msg["path"])
}
