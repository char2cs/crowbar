//go:build integration

package terminal_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain is the integration test harness entry point for the terminal package.
func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

// TerminalSuite exercises workspace-scoped terminal session and profile REST
// endpoints, the TerminalSessionDTO lifecycle broadcaster, and the PTY WebSocket
// route at the hierarchical .../terminals/:sessionId/ws path.
type TerminalSuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
	wsID     string
}

// SetupTest creates a fresh Env, imports a repo, and creates a workspace before
// each test.
func (s *TerminalSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "terminal", "")
	s.wsID = s.Env.CreateWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/terminal")
}

// base returns the workspace-scoped route prefix for the suite's workspace.
func (s *TerminalSuite) base() string {
	return "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID +
		"/workspaces/" + s.wsID
}

// TestTerminalSuite is the testify suite entry point for terminal integration tests.
func TestTerminalSuite(
	t *testing.T,
) {
	suite.Run(
		t,
		new(TerminalSuite),
	)
}

// TestTerminal_Create201ThenSessionDTOOverWS verifies a terminal session creates
// (201 {sessionId}) and the TerminalSessionDTO{status:"active"} lifecycle frame
// arrives on the workspace-scoped terminals WS (spec §10, D2).
func (s *TerminalSuite) TestTerminal_Create201ThenSessionDTOOverWS() {
	t := s.T()

	watcher := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	resp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, resp, http.StatusCreated)

	var body map[string]any
	kit.DecodeEnvData(t, resp, &body)
	sessionID, ok := body["sessionId"].(string)
	s.Require().True(ok, "sessionId must be a string")
	s.Require().NotEmpty(sessionID)

	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "active"
	})
	s.Assert().Equal(s.wsID, msg["workspaceId"])
	s.Assert().Equal(s.imported.RepoID, msg["repoId"])
	s.Assert().Equal(s.imported.ProjectID, msg["projectId"])

	// Kill the session and block on its "ended" frame so the spawned PTY (whose
	// CWD is the worktree) is reaped before the test's TempDir is removed —
	// otherwise the live shell holds the per-workspace dir busy and RemoveAll
	// flakes with "directory not empty".
	kill := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	kill.Body.Close()
	watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
}

// TestRegression_TerminalSession_LifecycleBroadcast proves the §10 terminal
// lifecycle topic: creating a session broadcasts an "active" TerminalSessionDTO
// and killing it broadcasts an "ended" one, both on the workspace-scoped
// terminals WS, carrying the hierarchical project/repo/workspace ids.
func (s *TerminalSuite) TestRegression_TerminalSession_LifecycleBroadcast() {
	t := s.T()

	watcher := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	active := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "active"
	})
	s.Assert().Equal(s.wsID, active["workspaceId"])

	killResp := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	defer killResp.Body.Close()
	kit.RequireStatus(t, killResp, http.StatusAccepted)

	ended := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
	s.Assert().Equal(sessionID, ended["id"])
}

// TestTerminal_DeleteScopedRoute verifies a session is killed via the
// workspace-scoped DELETE route (202) and an "ended" lifecycle frame broadcasts.
func (s *TerminalSuite) TestTerminal_DeleteScopedRoute() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	watcher := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	killResp := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	defer killResp.Body.Close()
	kit.RequireStatus(t, killResp, http.StatusAccepted)

	msg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
	s.Assert().Equal(sessionID, msg["id"])
}

// TestTerminal_KillUnknownSessionReturns404 verifies kill of unknown session is 404.
func (s *TerminalSuite) TestTerminal_KillUnknownSessionReturns404() {
	t := s.T()

	resp := s.Env.DELETE(t, s.base()+"/terminals/does-not-exist")
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
}

// TestTerminal_CreateForUnknownWorkspaceReturns404 verifies 404 on bad wsId.
func (s *TerminalSuite) TestTerminal_CreateForUnknownWorkspaceReturns404() {
	t := s.T()

	bad := "/v0/projects/" + s.imported.ProjectID +
		"/repos/" + s.imported.RepoID + "/workspaces/no-such-ws/terminals"
	resp := s.Env.POST(t, bad, map[string]any{})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
}

// TestTerminal_ProfileCRUD exercises the terminal profile REST endpoints (a
// global user setting, top-level under /v0/settings).
func (s *TerminalSuite) TestTerminal_ProfileCRUD() {
	t := s.T()

	createResp := s.Env.POST(t, "/v0/settings/terminal/profiles", map[string]any{
		"name":  "My Shell",
		"shell": "/bin/bash",
	})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	id, ok := created["id"].(string)
	s.Require().True(ok)
	s.Assert().NotEmpty(id)

	getResp := s.Env.GET(t, "/v0/settings/terminal/profiles/"+id)
	kit.RequireStatus(t, getResp, http.StatusOK)
	var got map[string]any
	kit.DecodeEnvData(t, getResp, &got)
	s.Assert().Equal("My Shell", got["name"])

	listResp := s.Env.GET(t, "/v0/settings/terminal/profiles")
	kit.RequireStatus(t, listResp, http.StatusOK)
	var list []map[string]any
	kit.DecodeEnvData(t, listResp, &list)
	s.Assert().NotEmpty(list)

	updateResp := s.Env.PUT(t, "/v0/settings/terminal/profiles/"+id, map[string]any{
		"name":  "Updated Shell",
		"shell": "/bin/bash",
	})
	defer updateResp.Body.Close()
	kit.RequireStatus(t, updateResp, http.StatusOK)

	deleteResp := s.Env.DELETE(t, "/v0/settings/terminal/profiles/"+id)
	defer deleteResp.Body.Close()
	kit.RequireStatus(t, deleteResp, http.StatusNoContent)

	getAfterDelete := s.Env.GET(t, "/v0/settings/terminal/profiles/"+id)
	defer getAfterDelete.Body.Close()
	s.Assert().Equal(http.StatusNotFound, getAfterDelete.StatusCode)
}

// TestRegression_TerminalSession_RealStateInListAndEndedBroadcast verifies the
// lifecycle wire-protocol Phase 2 on the REST + broadcast surface:
//
//   - GET /terminals (ListSessions) reports the engine's real state for each
//     session; a newly created session with no attached clients is "detached"
//     (live but no clients), NOT the hardcoded "active" of the previous impl.
//   - The "ended" lifecycle frame carries an exitCode field (may be nil when
//     the kill is via SIGKILL and the exit code is unknown).
func (s *TerminalSuite) TestRegression_TerminalSession_RealStateInListAndEndedBroadcast() {
	t := s.T()

	watcher := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	// Create a session — the POST broadcasts "active" and returns 201.
	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	// Wait for the "active" broadcast from the POST handler.
	watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "active"
	})

	// GET /terminals should now return the real engine state. With no attached
	// clients, StateOf returns "detached" (live PTY, no clients) — NOT "active".
	listResp := s.Env.GET(t, s.base()+"/terminals")
	kit.RequireStatus(t, listResp, http.StatusOK)
	var sessions []map[string]any
	kit.DecodeEnvData(t, listResp, &sessions)
	var found map[string]any
	for _, sess := range sessions {
		if sess["id"] == sessionID {
			found = sess
			break
		}
	}
	s.Require().NotNil(found, "created session must appear in ListSessions")
	s.Assert().Equal("detached", found["status"],
		"ListSessions must report real engine state ('detached' for no-client live session)")

	// Kill → "ended" broadcast. The exitCode field may or may not be present
	// (SIGKILL → -1 → omitted); we assert the status and presence of the frame.
	killResp := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	defer killResp.Body.Close()
	kit.RequireStatus(t, killResp, http.StatusAccepted)

	endedMsg := watcher.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
	s.Assert().Equal(sessionID, endedMsg["id"])
	s.Assert().Equal(s.wsID, endedMsg["workspaceId"])
}

// TestTerminal_PTYWSAtScopedPath verifies the raw PTY WebSocket connects at the
// hierarchical .../terminals/:sessionId/ws path and streams JSON text frames
// {sessionId, data, isInput}.
func (s *TerminalSuite) TestTerminal_PTYWSAtScopedPath() {
	t := s.T()

	lifecycle := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, ok := createBody["sessionId"].(string)
	s.Require().True(ok, "sessionId must be a string")

	ws := s.Env.DialTerminalPTY(t, s.imported.ProjectID, s.imported.RepoID, s.wsID, sessionID)
	frame := ws.ReadMsg(t, 10*time.Second)
	s.Require().Contains(frame, "sessionId", "wire frame must contain sessionId field")
	s.Assert().Equal(sessionID, frame["sessionId"], "frame sessionId must match")
	s.Require().Contains(frame, "data", "wire frame must contain data field")

	// Kill and block on the "ended" frame so the PTY shell (CWD = worktree) is
	// reaped before TempDir teardown.
	kill := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	kill.Body.Close()
	lifecycle.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
}
