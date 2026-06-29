//go:build integration

package terminal_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

// TestRegression_TerminalSession_HighThroughputOutputIntact proves that the
// writePump output-coalescing optimization (it merges PTY frames already queued
// on the client channel into a single WebSocket message under load) preserves
// the byte stream exactly: a large deterministic burst is delivered in full and
// in order, with no dropped, duplicated, or reordered bytes. Guards the
// coalescing change in terminalEngine.writePump.
func (s *TerminalSuite) TestRegression_TerminalSession_HighThroughputOutputIntact() {
	t := s.T()

	lifecycle := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, _ := createBody["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws := s.Env.DialTerminalPTY(t, s.imported.ProjectID, s.imported.RepoID, s.wsID, sessionID)

	// Wait for the initial shell prompt so the PTY is confirmed live before the
	// burst (otherwise the command can race the shell's startup).
	ws.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return len(data) > 0
	})

	// Drive a large deterministic burst. The completion marker is assembled from
	// a shell variable so the literal "BURST_COMPLETE_7d1c" appears ONLY in the
	// command's OUTPUT, never in the echoed command line — otherwise ReadUntil
	// would stop on the echo before the burst arrived.
	ws.SendJSON(t, map[string]any{
		"data": "M=BURST_COMPLETE; seq 1 3000; echo \"${M}_7d1c\"\n",
	})

	var sb strings.Builder
	ws.ReadUntil(t, 20*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		sb.WriteString(data)
		return strings.Contains(sb.String(), "BURST_COMPLETE_7d1c")
	})

	// Strip the PTY's ONLCR carriage returns so the line checks are termios-agnostic.
	out := strings.ReplaceAll(sb.String(), "\r", "")

	// Early, middle, and tail of the ascending run must each be present as
	// consecutive newline-delimited sequences. These multi-line substrings cannot
	// come from the echoed `seq 1 3000` command and would break if coalescing
	// dropped, duplicated, or reordered any frame boundary. (We anchor on runs
	// with a guaranteed leading "\n" — the very first "1" follows a terminal
	// bell from the shell's OSC title sequence, not a newline.)
	s.Assert().Contains(out, "\n10\n11\n12\n13\n14\n15\n", "start of burst must arrive intact and in order")
	s.Assert().Contains(out, "\n1500\n1501\n1502\n1503\n1504\n1505\n", "middle of burst must arrive intact and in order")
	s.Assert().Contains(out, "\n2996\n2997\n2998\n2999\n3000\n", "end of burst must arrive intact and in order")

	// The completion marker must arrive AFTER the full burst — proves the entire
	// ordered stream was delivered before the trailing echo, with nothing lost.
	idxTail := strings.Index(out, "\n3000\n")
	idxMarker := strings.Index(out, "BURST_COMPLETE_7d1c")
	s.Require().GreaterOrEqual(idxTail, 0, "tail of burst (3000) must be present")
	s.Assert().Greater(idxMarker, idxTail, "completion marker must follow the full burst")

	// Cleanup: kill and block on "ended" so the PTY is reaped before TempDir teardown.
	kill := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	kill.Body.Close()
	lifecycle.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
}

// TestRegression_TerminalSession_MultibyteOutputNotCorrupted proves the writePump
// UTF-8 holdback: a large burst of multi-byte glyphs (box-drawing U+2500, which a
// TUI like Claude Code draws constantly) is delivered with every rune intact and
// ZERO U+FFFD replacement characters — even though the bytes straddle the PTY read
// buffer and message-coalescing boundaries. Without the holdback, json.Marshal
// corrupts any rune split across a message into U+FFFD (the on-screen "???"/box).
func (s *TerminalSuite) TestRegression_TerminalSession_MultibyteOutputNotCorrupted() {
	t := s.T()

	lifecycle := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, _ := createBody["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws := s.Env.DialTerminalPTY(t, s.imported.ProjectID, s.imported.RepoID, s.wsID, sessionID)
	ws.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return len(data) > 0
	})

	// Emit ~120 KB of the 3-byte rune U+2500 (well past the 64 KB read buffer, so
	// runes are guaranteed to straddle read+coalesce boundaries), then a marker
	// assembled from a shell var so the literal only appears in the OUTPUT.
	ws.SendJSON(t, map[string]any{
		"data": "M=BURSTDONE; yes ─ | head -n 40000 | tr -d '\\n'; echo \"${M}_u8\"\n",
	})

	var sb strings.Builder
	ws.ReadUntil(t, 25*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		sb.WriteString(data)
		return strings.Contains(sb.String(), "BURSTDONE_u8")
	})
	out := sb.String()

	// The headline invariant: not a single replacement character anywhere.
	s.Assert().NotContains(out, "�", "multi-byte glyphs must not be corrupted to U+FFFD")
	// And the run actually arrived: at least the 40000 emitted box-drawing runes.
	s.Assert().GreaterOrEqual(strings.Count(out, "─"), 40000, "all box-drawing runes must arrive intact")

	kill := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	kill.Body.Close()
	lifecycle.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
}

// TestRegression_TerminalSession_ReAttachReassertsLiveModes proves the DEC
// private-mode preamble: a client attaching mid-session to a LIVE app that enabled
// mouse tracking receives that mode re-asserted, even though the replay sanitizer
// strips the mode bytes from the scrollback. Without it, a workspace switch (which
// disposes + recreates xterm) leaves the new terminal in default modes and breaks
// Claude Code's mouse-wheel and arrow-key scrolling.
func (s *TerminalSuite) TestRegression_TerminalSession_ReAttachReassertsLiveModes() {
	t := s.T()

	lifecycle := s.Env.DialTerminals(t, s.imported.ProjectID, s.imported.RepoID, s.wsID)
	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws1 := s.Env.DialTerminalPTY(t, s.imported.ProjectID, s.imported.RepoID, s.wsID, sessionID)
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		return len(d) > 0
	})

	// Enable mouse tracking (?1000h — which the sanitizer strips from scrollback),
	// emit an ASCII marker right after it, then keep a foreground child alive
	// (sleep) so the session is NOT idle. Wait for the marker (accumulated, so a
	// frame split can't hide it): seeing it proves printf ran, so the pump has
	// observed ?1000h, and the shell has moved on to the foreground sleep.
	ws1.SendJSON(t, map[string]any{"data": "printf '\\033[?1000hMODESET_OK'; sleep 30\n"})
	var ws1buf strings.Builder
	ws1.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		ws1buf.WriteString(d)
		return strings.Contains(ws1buf.String(), "MODESET_OK")
	})

	// Re-attach: a SECOND client. Its scrollback snapshot has ?1000h stripped, so
	// if ?1000h appears it can ONLY be the mode preamble re-asserting it.
	ws2 := s.Env.DialTerminalPTY(t, s.imported.ProjectID, s.imported.RepoID, s.wsID, sessionID)
	var sb strings.Builder
	reasserted := false
	ws2.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		sb.WriteString(d)
		if strings.Contains(sb.String(), "\x1b[?1000h") {
			reasserted = true
			return true
		}
		return false
	})
	s.Assert().True(reasserted,
		"re-attach must re-assert mouse tracking ?1000h via the mode preamble (the sanitizer strips it from the replayed scrollback)")

	kill := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	kill.Body.Close()
	lifecycle.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
}

// ---------------------------------------------------------------------------
// TestRegression_TerminalSession_RestartRoundTrip — full REST/WS restart test
// ---------------------------------------------------------------------------

// TestRegression_TerminalSession_RestartRoundTrip is the end-to-end
// daemon-restart integration test at REST/WS fidelity. It uses the real gin
// router, real SQLite-backed GORM stores, and real disk-backed scrollback
// persistence to prove the headline guarantee:
//
//   Sessions survive a simulated daemon restart; scrollback (including known
//   output driven into the session before shutdown) is replayed when the client
//   dials the PTY WS on the fresh server, and GET /terminals shows the session
//   as "suspended" before the first attach.
//
// Sequence (mirrors the daemon restart path exactly):
//  1. env1: import repo, create workspace, POST /terminals to create a session.
//  2. env1: dial the PTY WS, wait for initial shell output (confirms live PTY).
//  3. env1: send "echo restart-crossboundary-marker" via the PTY WS.
//  4. env1: wait for the marker in the PTY output stream.
//  5. env1.ShutdownTerminal(): flush all live sessions to disk with state="suspended".
//  6. env1.Close(t): release adapter SQLite connections (release file locks).
//  7. env2 := BuildEnvAt(same homeDir): restores sessions synchronously via
//     startRestoreTerminalSessions in app.New.
//  8. env2: GET /terminals → session must appear with status "suspended".
//  9. env2: dial the PTY WS → triggers transparent restore.
// 10. Assert "restart-crossboundary-marker" appears in the replayed frames.
func TestRegression_TerminalSession_RestartRoundTrip(t *testing.T) {
	// A homeDir shared between env1 and env2 is what makes the SQLite session
	// rows and the .buf scrollback files persist across the simulated restart.
	homeDir := kit.TempHomeForTest(t)

	// ---- env1: create session and drive known output. ----
	env1 := kit.BuildEnvAt(t, homeDir)
	imported := env1.ImportRepo(t, "restart-terminal", "")
	wsID := env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/restart")
	base := "/v0/projects/" + imported.ProjectID + "/repos/" + imported.RepoID + "/workspaces/" + wsID

	// Dial the lifecycle WS before creating the session so we never miss the
	// "active" broadcast.
	lifecycle1 := env1.DialTerminals(t, imported.ProjectID, imported.RepoID, wsID)

	createResp := env1.POST(t, base+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, _ := createBody["sessionId"].(string)
	require.NotEmpty(t, sessionID, "POST /terminals must return a sessionId")

	// Wait for the "active" lifecycle broadcast so the session is confirmed live.
	lifecycle1.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "active"
	})

	// Dial the raw PTY WS and wait for the shell's initial output (prompt).
	// This confirms the PTY subprocess is running and the ring buffer has content.
	ptyWS1 := env1.DialTerminalPTY(t, imported.ProjectID, imported.RepoID, wsID, sessionID)
	ptyWS1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return len(data) > 0
	})

	// Send a specific marker command via the PTY WS so we can prove scrollback
	// replay contains pre-restart content (not just a fresh shell prompt).
	ptyWS1.SendJSON(t, map[string]any{"data": "echo restart-crossboundary-marker\n"})

	// Wait for the marker to arrive in the PTY output frames.
	ptyWS1.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return strings.Contains(data, "restart-crossboundary-marker")
	})

	// ShutdownTerminal flushes all live sessions (including the active PTY WS
	// client) to disk as state="suspended", then kills the PTY processes. The
	// PTY WS conn from env1 will receive errors after this point (the session is
	// dead), but that is handled by t.Cleanup's conn.Close().
	env1.ShutdownTerminal()

	// Release env1's adapter SQLite connections so env2 can open the same DB
	// files. The deferred t.Cleanup for env1 will call Close a second time;
	// that is safe — the underlying sql.DB is already closed and Close is idempotent.
	env1.Close(t)

	// ---- env2: fresh server over the same homeDir → restores sessions. ----
	//
	// app.New calls startRestoreTerminalSessions synchronously, so by the time
	// BuildEnvAt returns the session is already a suspended placeholder in eng2's
	// registry.
	env2 := kit.BuildEnvAt(t, homeDir)

	// GET /terminals on env2 must list the restored session as "suspended".
	listResp := env2.GET(t, base+"/terminals")
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
	require.NotNil(t, found,
		"GET /terminals on env2 must list the restored session (id=%s)", sessionID)
	require.Equal(t, "suspended", found["status"],
		"restored session must have status='suspended' before first attach")

	// Dial the PTY WS on env2 — this triggers the transparent restore path:
	// restore() spawns a fresh shell with the saved CWD and replays the
	// scrollback (including our marker) as the first frame.
	ptyWS2 := env2.DialTerminalPTY(t, imported.ProjectID, imported.RepoID, wsID, sessionID)

	// The replayed scrollback must contain the pre-restart marker.
	ptyWS2.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return strings.Contains(data, "restart-crossboundary-marker")
	})

	// Cleanup: kill the restored session so the PTY subprocess is reaped before
	// TempDir teardown (avoids "directory not empty" flakes on some platforms).
	lifecycle2 := env2.DialTerminals(t, imported.ProjectID, imported.RepoID, wsID)
	killResp := env2.DELETE(t, base+"/terminals/"+sessionID)
	killResp.Body.Close()
	lifecycle2.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		return m["id"] == sessionID && m["status"] == "ended"
	})
}
