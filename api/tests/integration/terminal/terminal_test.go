//go:build integration

package terminal_test

import (
	"net/http"
	"strings"
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

// TerminalSuite exercises terminal session and profile REST endpoints plus the PTY WebSocket route.
type TerminalSuite struct {
	kit.IntegrationSuite
	repoPath string
	wsID     string
}

// SetupTest creates a fresh Env and registers a workspace before each test.
func (s *TerminalSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.repoPath = kit.InitRepo(s.T())

	repoResp := s.Env.POST(s.T(), "/v0/repos", map[string]any{
		"id":        "r1",
		"projectId": "p1",
		"name":      "repo",
		"path":      s.repoPath,
	})
	kit.RequireStatus(s.T(), repoResp, http.StatusCreated)
	repoResp.Body.Close()

	resp := s.Env.POST(
		s.T(),
		"/v0/workspaces",
		map[string]any{
			"repoId": "r1",
			"branch": kit.BranchName(s.T(), s.repoPath),
		},
	)
	kit.RequireStatus(s.T(), resp, http.StatusCreated)
	s.wsID = kit.MutationID(s.T(), resp)
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

// TestTerminal_CreateAndKill verifies a terminal session can be created and killed.
func (s *TerminalSuite) TestTerminal_CreateAndKill() {
	t := s.T()

	resp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/terminals",
		map[string]any{},
	)
	kit.RequireStatus(
		t,
		resp,
		http.StatusCreated,
	)

	var body map[string]any
	kit.DecodeEnvData(
		t,
		resp,
		&body,
	)
	sessionID, ok := body["sessionId"].(string)
	s.Require().True(
		ok,
		"sessionId must be a string",
	)
	s.Assert().NotEmpty(sessionID)

	killResp := s.Env.DELETE(
		t,
		"/v0/terminals/"+sessionID,
	)
	defer killResp.Body.Close()
	kit.RequireStatus(
		t,
		killResp,
		http.StatusOK,
	)
}

// TestTerminal_KillUnknownSessionReturns404 verifies kill of unknown session is 404.
func (s *TerminalSuite) TestTerminal_KillUnknownSessionReturns404() {
	t := s.T()

	resp := s.Env.DELETE(
		t,
		"/v0/terminals/does-not-exist",
	)
	defer resp.Body.Close()
	s.Assert().Equal(
		http.StatusNotFound,
		resp.StatusCode,
	)
}

// TestTerminal_CreateForUnknownWorkspaceReturns404 verifies 404 on bad wsId.
func (s *TerminalSuite) TestTerminal_CreateForUnknownWorkspaceReturns404() {
	t := s.T()

	resp := s.Env.POST(
		t,
		"/v0/workspaces/no-such-ws/terminals",
		map[string]any{},
	)
	defer resp.Body.Close()
	s.Assert().Equal(
		http.StatusNotFound,
		resp.StatusCode,
	)
}

// TestTerminal_ProfileCRUD exercises the terminal profile REST endpoints.
func (s *TerminalSuite) TestTerminal_ProfileCRUD() {
	t := s.T()

	// Create
	createResp := s.Env.POST(
		t,
		"/v0/settings/terminal/profiles",
		map[string]any{
			"name":  "My Shell",
			"shell": "/bin/bash",
		},
	)
	kit.RequireStatus(
		t,
		createResp,
		http.StatusCreated,
	)
	var created map[string]any
	kit.DecodeEnvData(
		t,
		createResp,
		&created,
	)
	id, ok := created["id"].(string)
	s.Require().True(ok)
	s.Assert().NotEmpty(id)

	// Get
	getResp := s.Env.GET(
		t,
		"/v0/settings/terminal/profiles/"+id,
	)
	kit.RequireStatus(
		t,
		getResp,
		http.StatusOK,
	)
	var got map[string]any
	kit.DecodeEnvData(
		t,
		getResp,
		&got,
	)
	s.Assert().Equal(
		"My Shell",
		got["name"],
	)

	// List
	listResp := s.Env.GET(
		t,
		"/v0/settings/terminal/profiles",
	)
	kit.RequireStatus(
		t,
		listResp,
		http.StatusOK,
	)
	var list []map[string]any
	kit.DecodeEnvData(
		t,
		listResp,
		&list,
	)
	s.Assert().NotEmpty(list)

	// Update
	updateResp := s.Env.PUT(
		t,
		"/v0/settings/terminal/profiles/"+id,
		map[string]any{
			"name":  "Updated Shell",
			"shell": "/bin/bash",
		},
	)
	defer updateResp.Body.Close()
	kit.RequireStatus(
		t,
		updateResp,
		http.StatusOK,
	)

	// Delete
	deleteResp := s.Env.DELETE(
		t,
		"/v0/settings/terminal/profiles/"+id,
	)
	defer deleteResp.Body.Close()
	kit.RequireStatus(
		t,
		deleteResp,
		http.StatusNoContent,
	)

	// Verify gone
	getAfterDelete := s.Env.GET(
		t,
		"/v0/settings/terminal/profiles/"+id,
	)
	defer getAfterDelete.Body.Close()
	s.Assert().Equal(
		http.StatusNotFound,
		getAfterDelete.StatusCode,
	)
}

// TestTerminal_WSConnectionEstablishes verifies that a terminal WS connection
// can be established to an active session. The terminal WS streams JSON text
// frames with envelope {sessionId, data, isInput}.
func (s *TerminalSuite) TestTerminal_WSConnectionEstablishes() {
	t := s.T()

	createResp := s.Env.POST(
		t,
		"/v0/workspaces/"+s.wsID+"/terminals",
		map[string]any{},
	)
	kit.RequireStatus(
		t,
		createResp,
		http.StatusCreated,
	)

	var createBody map[string]any
	kit.DecodeEnvData(
		t,
		createResp,
		&createBody,
	)
	sessionID, ok := createBody["sessionId"].(string)
	s.Require().True(
		ok,
		"sessionId must be a string",
	)
	t.Cleanup(func() {
		resp := s.Env.DELETE(
			t,
			"/v0/terminals/"+sessionID,
		)
		resp.Body.Close()
	})

	wsURL := "ws" + strings.TrimPrefix(s.Env.URL, "http") + "/v0/ws/terminals/" + sessionID
	ws := kit.Dial(
		t,
		wsURL,
	)
	frame := ws.ReadMsg(
		t,
		10*time.Second,
	)
	s.Require().Contains(
		frame,
		"sessionId",
		"wire frame must contain sessionId field",
	)
	s.Assert().Equal(
		sessionID,
		frame["sessionId"],
		"frame sessionId must match the created session",
	)
	s.Require().Contains(
		frame,
		"data",
		"wire frame must contain data field",
	)
}
