//go:build integration

package terminal_test

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/vt"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/char2cs/crowbar/api/tests/kit"
)

var deviceQueryOrReply = regexp.MustCompile("\x1b\\[[0-9;?>=]*[cn]")

func gridRowText(
	emu *vt.Emulator,
	cols int,
	y int,
) string {
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := emu.CellAt(x, y)
		if c == nil || c.Content == "" {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(c.Content)
	}
	return b.String()
}

func gridContains(
	emu *vt.Emulator,
	cols int,
	rows int,
	substr string,
) bool {
	for y := 0; y < rows; y++ {
		if strings.Contains(gridRowText(emu, cols, y), substr) {
			return true
		}
	}
	return false
}

func scrollbackRowText(
	emu *vt.Emulator,
	cols int,
	y int,
) string {
	var b strings.Builder
	for x := 0; x < cols; x++ {
		c := emu.ScrollbackCellAt(x, y)
		if c == nil || c.Content == "" {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(c.Content)
	}
	return b.String()
}

func emuHasLine(
	emu *vt.Emulator,
	cols int,
	rows int,
	line string,
) bool {
	for y := 0; y < rows; y++ {
		if strings.TrimRight(gridRowText(emu, cols, y), " ") == line {
			return true
		}
	}
	for y := 0; y < emu.ScrollbackLen(); y++ {
		if strings.TrimRight(scrollbackRowText(emu, cols, y), " ") == line {
			return true
		}
	}
	return false
}

func TestMain(
	m *testing.M,
) {
	kit.Main(m)
}

type TerminalSuite struct {
	kit.IntegrationSuite
	imported kit.ImportedRepo
	wsID     string
	chatID   string
}

func (s *TerminalSuite) SetupTest() {
	s.IntegrationSuite.SetupTest()
	s.imported = s.Env.ImportRepo(s.T(), "terminal", "")
	s.wsID = s.Env.CreateWorkspace(s.T(), s.imported.ProjectID, s.imported.RepoID, "feature/terminal")
	s.chatID = s.Env.OwningChatID(s.T(), s.wsID)
}

func (s *TerminalSuite) base() string {
	return "/v0/chats/" + s.chatID
}

func TestTerminalSuite(
	t *testing.T,
) {
	suite.Run(t, new(TerminalSuite))
}

func (s *TerminalSuite) TestTerminal_Create201ThenKill() {
	t := s.T()

	resp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, resp, http.StatusCreated)

	var body map[string]any
	kit.DecodeEnvData(t, resp, &body)
	sessionID, ok := body["sessionId"].(string)
	s.Require().True(ok, "sessionId must be a string")
	s.Require().NotEmpty(sessionID)

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestTerminal_DeleteScopedRoute() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	s.kill(t, sessionID)
	listResp := s.Env.GET(t, s.base()+"/terminals") //nolint:bodyclose // DecodeEnvData closes the body
	kit.RequireStatus(t, listResp, http.StatusOK)
	var sessions []map[string]any
	kit.DecodeEnvData(t, listResp, &sessions)
	for _, sess := range sessions {
		s.NotEqual(sessionID, sess["id"], "a killed session is no longer listed")
	}
}

func (s *TerminalSuite) TestTerminal_KillUnknownSessionReturns404() {
	t := s.T()

	resp := s.Env.DELETE(t, s.base()+"/terminals/does-not-exist")
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
}

func (s *TerminalSuite) TestTerminal_CreateForUnknownChatReturns404() {
	t := s.T()

	resp := s.Env.POST(t, "/v0/chats/no-such-chat/terminals", map[string]any{})
	defer resp.Body.Close()
	s.Assert().Equal(http.StatusNotFound, resp.StatusCode)
}

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

func (s *TerminalSuite) TestRegression_TerminalSession_RealStateInList() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

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

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestTerminal_PTYWSAtScopedPath() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, ok := createBody["sessionId"].(string)
	s.Require().True(ok, "sessionId must be a string")

	ws := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	frame := ws.ReadMsg(t, 10*time.Second)
	s.Require().Contains(frame, "data", "the first PTY frame must be output")
	s.Equal(true, frame["snapshot"], "the first frame after attach is the snapshot")

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestRegression_TerminalSession_HighThroughputOutputIntact() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, _ := createBody["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws := s.Env.DialTerminalPTY(t, s.chatID, sessionID)

	ws.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return len(data) > 0
	})

	ws.SendJSON(t, map[string]any{
		"data": "M=BURST_COMPLETE; seq 1 3000; echo \"${M}_7d1c\"\n",
	})

	const cols, rows = 80, 24
	emu := vt.NewEmulator(cols, rows)
	emu.SetScrollbackSize(10000)

	var acc strings.Builder
	frames := make([]string, 0, 512)
	ws.ReadUntil(t, 20*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		acc.WriteString(data)
		frames = append(frames, data)
		return strings.Contains(acc.String(), "BURST_COMPLETE_7d1c")
	})

	markerSeen := false
	for _, f := range frames {
		_, werr := emu.Write([]byte(f))
		s.Require().NoError(werr)
		if !markerSeen && emuHasLine(emu, cols, rows, "BURST_COMPLETE_7d1c") {
			markerSeen = true
			s.Assert().True(emuHasLine(emu, cols, rows, "3000"),
				"completion marker must not arrive before the tail of the burst (line 3000)")
		}
	}
	s.Require().True(markerSeen, "completion marker must appear in the reconstructed model output")

	s.Assert().True(emuHasLine(emu, cols, rows, "10"), "start of burst (line 10) must be reconstructed intact")
	s.Assert().True(emuHasLine(emu, cols, rows, "1500"), "middle of burst (line 1500) must be reconstructed intact")
	s.Assert().True(emuHasLine(emu, cols, rows, "3000"), "end of burst (line 3000) must be reconstructed intact")

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestRegression_TerminalSession_MultibyteOutputNotCorrupted() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, _ := createBody["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	ws.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return len(data) > 0
	})

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

	s.Assert().NotContains(out, "�", "multi-byte glyphs must not be corrupted to U+FFFD")
	s.Assert().GreaterOrEqual(strings.Count(out, "─"), 40000, "all box-drawing runes must arrive intact")

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestRegression_TerminalSession_ReAttachReassertsLiveModes() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws1 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		return len(d) > 0
	})

	ws1.SendJSON(t, map[string]any{"data": "printf '\\033[?1000hMODESET_OK'; sleep 30\n"})
	var ws1buf strings.Builder
	ws1.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		ws1buf.WriteString(d)
		return strings.Contains(ws1buf.String(), "MODESET_OK")
	})

	ws2 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
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
		"re-attach must re-assert mouse tracking ?1000h via the serialized redraw")

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestRegression_TerminalSession_ReAttachPayloadIsCleanRedraw() {
	t := s.T()

	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws1 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		return len(d) > 0
	})

	ws1.SendJSON(t, map[string]any{
		"data": "printf '\\033]2;CleanTitle\\007\\033[?1000hSCREENMARK_7f3'; sleep 30\n",
	})
	var ws1buf strings.Builder
	ws1.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		ws1buf.WriteString(d)
		return strings.Contains(ws1buf.String(), "\x1b[?1000h") &&
			strings.Contains(ws1buf.String(), "SCREENMARK_7f3")
	})

	ws2 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	frame := ws2.ReadMsg(t, 5*time.Second)
	p, _ := frame["data"].(string)
	s.Require().NotEmpty(p)
	s.Require().Contains(p, "SCREENMARK_7f3",
		"the first re-attach frame must be the full serialized redraw containing the marker")

	s.Assert().False(deviceQueryOrReply.MatchString(p),
		"serialized redraw must contain no DA/DSR device query or reply; got %q", p)

	s.Assert().Contains(p, ansi.ResetStyle,
		"serialized redraw must reset the pen to ground state")
	s.Assert().NotEqual(byte(0x1b), p[len(p)-1],
		"serialized redraw must not end on a bare ESC")
	s.Assert().False(strings.HasSuffix(p, "\x1b["),
		"serialized redraw must not end on an unterminated CSI")

	const cols, rows = 80, 24
	modes := map[int]bool{}
	title := ""
	fresh := vt.NewEmulator(cols, rows)
	fresh.SetCallbacks(vt.Callbacks{
		Title: func(s string) { title = s },
		EnableMode: func(md ansi.Mode) {
			if dm, ok := md.(ansi.DECMode); ok {
				modes[dm.Mode()] = true
			}
		},
		DisableMode: func(md ansi.Mode) {
			if dm, ok := md.(ansi.DECMode); ok {
				modes[dm.Mode()] = false
			}
		},
	})
	_, werr := fresh.Write([]byte(p))
	s.Require().NoError(werr)

	s.Assert().True(gridContains(fresh, cols, rows, "SCREENMARK_7f3"),
		"round-tripping the redraw into a fresh terminal must reproduce the on-screen marker")
	s.Assert().True(modes[1000],
		"round-trip must restore live mouse tracking ?1000")
	s.Assert().Equal("CleanTitle", title,
		"round-trip must restore the current window title")

	s.kill(t, sessionID)
}

func (s *TerminalSuite) newSessionWithLiveClient(
	t *testing.T,
) (string, *kit.WSWatcher) {
	createResp := s.Env.POST(t, s.base()+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var created map[string]any
	kit.DecodeEnvData(t, createResp, &created)
	sessionID, _ := created["sessionId"].(string)
	s.Require().NotEmpty(sessionID)

	ws1 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		return len(d) > 0
	})
	return sessionID, ws1
}

func (s *TerminalSuite) kill(
	t *testing.T,
	sessionID string,
) {
	t.Helper()
	resp := s.Env.DELETE(t, s.base()+"/terminals/"+sessionID)
	kit.RequireStatus(t, resp, http.StatusOK)
	require.NoError(t, resp.Body.Close())
}

func (s *TerminalSuite) TestRegression_ReattachNoQueryReplies() {
	t := s.T()

	sessionID, ws1 := s.newSessionWithLiveClient(t)

	ws1.SendJSON(t, map[string]any{
		"data": "printf '\\033[c\\033[6n\\033]10;?\\033\\\\\\033]2;QEXEC_3e7\\007'; sleep 30\n",
	})
	var ws1buf strings.Builder
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		ws1buf.WriteString(d)
		return strings.Contains(ws1buf.String(), "\x1b]2;QEXEC_3e7")
	})

	ws2 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	frame := ws2.ReadMsg(t, 5*time.Second)
	p, _ := frame["data"].(string)
	s.Require().NotEmpty(p)

	s.Assert().False(deviceQueryOrReply.MatchString(p),
		"serialized redraw must contain no DA/DSR query or reply; got %q", p)
	s.Assert().NotContains(p, "\x1b[6n", "redraw must not echo the historical DSR query")
	s.Assert().NotContains(p, "\x1b[c", "redraw must not echo the historical DA query")
	s.Assert().NotContains(p, "\x1b]10;",
		"serialized redraw must not echo the historical OSC-color query/reply; got %q", p)
	s.Assert().Contains(p, ansi.ResetStyle, "serialized redraw must reset the pen to ground state")

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestRegression_ReattachNoRawOSCTitleReplay() {
	t := s.T()

	sessionID, ws1 := s.newSessionWithLiveClient(t)

	ws1.SendJSON(t, map[string]any{
		"data": "printf '\\033]2;TitleAlpha\\007\\033]2;TitleBeta\\007\\033]2;TitleZeta_88\\007'; sleep 30\n",
	})
	var ws1buf strings.Builder
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		ws1buf.WriteString(d)
		return strings.Contains(ws1buf.String(), "\x1b]2;TitleZeta_88")
	})

	ws2 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	frame := ws2.ReadMsg(t, 5*time.Second)
	p, _ := frame["data"].(string)
	s.Require().NotEmpty(p)

	s.Assert().Equal(1, strings.Count(p, "\x1b]2;"),
		"serialized redraw must contain exactly one OSC-2 title (the current one), not a raw replay of every historical title; got %q", p)
	s.Assert().Contains(p, "\x1b]2;TitleZeta_88\x1b\\",
		"the single title must be the current one, ST-terminated")
	s.Assert().NotContains(p, "\x1b]2;TitleAlpha",
		"no stale historical title OSC may be replayed as a control sequence")
	s.Assert().NotContains(p, "\x1b]2;TitleBeta",
		"no stale historical title OSC may be replayed as a control sequence")

	const cols, rows = 80, 24
	title := ""
	fresh := vt.NewEmulator(cols, rows)
	fresh.SetCallbacks(vt.Callbacks{Title: func(s string) { title = s }})
	_, werr := fresh.Write([]byte(p))
	s.Require().NoError(werr)
	s.Assert().Equal("TitleZeta_88", title, "round-trip must restore exactly the current window title")

	s.kill(t, sessionID)
}

func (s *TerminalSuite) TestRegression_ReattachNoDanglingSequence() {
	t := s.T()

	sessionID, ws1 := s.newSessionWithLiveClient(t)

	ws1.SendJSON(t, map[string]any{
		"data": "M=PREMARK; printf \"${M}_44\\033]2;DANGLE_NOTERM_44\"; sleep 30\n",
	})
	var ws1buf strings.Builder
	ws1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		d, _ := m["data"].(string)
		ws1buf.WriteString(d)
		return strings.Contains(ws1buf.String(), "PREMARK_44")
	})

	ws2 := s.Env.DialTerminalPTY(t, s.chatID, sessionID)
	frame := ws2.ReadMsg(t, 5*time.Second)
	p, _ := frame["data"].(string)
	s.Require().NotEmpty(p)

	s.Require().Contains(p, "PREMARK_44",
		"re-attach redraw must reproduce the rendered marker (proving printf executed and the dangling OSC was buffered)")

	s.Assert().Contains(p, ansi.ResetStyle, "redraw must reset the pen to ground state")
	s.Assert().NotContains(p, "DANGLE_NOTERM_44\x1b\\",
		"the dangling OSC must not be emitted as a (falsely) ST-terminated title")
	s.Assert().NotContains(p, "DANGLE_NOTERM_44\x07",
		"the dangling OSC must not be emitted as a (falsely) BEL-terminated title")

	s.kill(t, sessionID)
}

func TestRegression_TerminalSession_RestartRoundTrip(t *testing.T) {
	homeDir := kit.TempHomeForTest(t)

	env1 := kit.BuildEnvAt(t, homeDir)
	imported := env1.ImportRepo(t, "restart-terminal", "")
	wsID := env1.CreateWorkspace(t, imported.ProjectID, imported.RepoID, "feature/restart")
	chatID := env1.OwningChatID(t, wsID)
	base := "/v0/chats/" + chatID

	createResp := env1.POST(t, base+"/terminals", map[string]any{})
	kit.RequireStatus(t, createResp, http.StatusCreated)
	var createBody map[string]any
	kit.DecodeEnvData(t, createResp, &createBody)
	sessionID, _ := createBody["sessionId"].(string)
	require.NotEmpty(t, sessionID, "POST /terminals must return a sessionId")

	ptyWS1 := env1.DialTerminalPTY(t, chatID, sessionID)
	ptyWS1.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return len(data) > 0
	})

	ptyWS1.SendJSON(t, map[string]any{"data": "echo restart-crossboundary-marker\n"})

	ptyWS1.ReadUntil(t, 5*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return strings.Contains(data, "restart-crossboundary-marker")
	})

	env1.ShutdownTerminal()

	env1.Close(t)

	env2 := kit.BuildEnvAt(t, homeDir)

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

	ptyWS2 := env2.DialTerminalPTY(t, chatID, sessionID)

	ptyWS2.ReadUntil(t, 10*time.Second, func(m map[string]any) bool {
		data, _ := m["data"].(string)
		return strings.Contains(data, "restart-crossboundary-marker")
	})

	killResp := env2.DELETE(t, base+"/terminals/"+sessionID)
	kit.RequireStatus(t, killResp, http.StatusOK)
	require.NoError(t, killResp.Body.Close())
}
