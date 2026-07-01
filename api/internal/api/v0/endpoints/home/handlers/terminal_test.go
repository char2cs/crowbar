package handlers_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/home/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

// ── mockTerminalEngine — a controllable TerminalEngine double ────────────

type mockTerminalEngine struct{ mock.Mock }

func (m *mockTerminalEngine) Create(
	ctx context.Context, workspaceID, workspaceDir string, prof *domain.TerminalProfile,
) (string, error) {
	args := m.Called(ctx, workspaceID, workspaceDir, prof)
	return args.String(0), args.Error(1)
}

func (m *mockTerminalEngine) Kill(ctx context.Context, sessionID string) error {
	args := m.Called(ctx, sessionID)
	return args.Error(0)
}

func (m *mockTerminalEngine) ListSessionsForWorkspace(workspaceID string) []string {
	args := m.Called(workspaceID)
	sessions, _ := args.Get(0).([]string)
	return sessions
}

func (m *mockTerminalEngine) SessionExists(ctx context.Context, sessionID string) bool {
	args := m.Called(ctx, sessionID)
	return args.Bool(0)
}

func (m *mockTerminalEngine) Attach(ctx context.Context, sessionID string, conn handlers.WSConn) error {
	args := m.Called(ctx, sessionID, conn)
	return args.Error(0)
}

func homeTerminalWorkspaceReader(t *testing.T, projectID, wsID string) *mockHomeReader {
	t.Helper()
	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, projectID).Return(domain.Workspace{
		ID:           wsID,
		ProjectID:    projectID,
		Kind:         domain.WorkspaceKindHome,
		WorktreePath: "/projects/" + projectID,
	}, nil)
	return reader
}

// ── ListTerminals ──────────────────────────────────────────────────────

func TestListTerminals_ReturnsSessions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("ListSessionsForWorkspace", "ws-1").Return([]string{"sess1", "sess2"})

	h := handlers.New(reader, nil, nil, eng)
	r.GET("/projects/:projectId/home/terminals", h.ListTerminals)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/terminals", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, []string{"sess1", "sess2"}, env.Data)
	eng.AssertExpectations(t)
}

func TestListTerminals_NilSessions_ReturnsEmptyArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("ListSessionsForWorkspace", "ws-1").Return(nil)

	h := handlers.New(reader, nil, nil, eng)
	r.GET("/projects/:projectId/home/terminals", h.ListTerminals)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/terminals", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data []string `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.NotNil(t, env.Data)
	require.Empty(t, env.Data)
}

func TestListTerminals_WorkspaceResolutionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-x").
		Return(domain.Workspace{}, errors.New("boom"))
	eng := &mockTerminalEngine{}

	h := handlers.New(reader, nil, nil, eng)
	r.GET("/projects/:projectId/home/terminals", h.ListTerminals)

	rec := doReq(r, http.MethodGet, "/projects/proj-x/home/terminals", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	eng.AssertNotCalled(t, "ListSessionsForWorkspace", mock.Anything)
}

// ── CreateTerminal ─────────────────────────────────────────────────────

func TestCreateTerminal_Returns201WithSessionID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("Create", mock.Anything, "ws-1", "/projects/proj-1", (*domain.TerminalProfile)(nil)).
		Return("sess-new", nil)

	h := handlers.New(reader, nil, nil, eng)
	r.POST("/projects/:projectId/home/terminals", h.CreateTerminal)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/terminals", nil)
	require.Equal(t, http.StatusCreated, rec.Code)

	var env struct {
		Data struct {
			SessionID string `json:"sessionId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Equal(t, "sess-new", env.Data.SessionID)
	eng.AssertExpectations(t)
}

func TestCreateTerminal_EngineError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("Create", mock.Anything, "ws-1", "/projects/proj-1", (*domain.TerminalProfile)(nil)).
		Return("", errors.New("spawn failed"))

	h := handlers.New(reader, nil, nil, eng)
	r.POST("/projects/:projectId/home/terminals", h.CreateTerminal)

	rec := doReq(r, http.MethodPost, "/projects/proj-1/home/terminals", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCreateTerminal_WorkspaceResolutionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-x").
		Return(domain.Workspace{}, errors.New("boom"))
	eng := &mockTerminalEngine{}

	h := handlers.New(reader, nil, nil, eng)
	r.POST("/projects/:projectId/home/terminals", h.CreateTerminal)

	rec := doReq(r, http.MethodPost, "/projects/proj-x/home/terminals", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	eng.AssertNotCalled(t, "Create", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ── KillTerminal ───────────────────────────────────────────────────────

func TestKillTerminal_WorkspaceResolutionFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := &mockHomeReader{}
	reader.On("GetHomeForProject", mock.Anything, "proj-x").
		Return(domain.Workspace{}, errors.New("boom"))
	eng := &mockTerminalEngine{}

	h := handlers.New(reader, nil, nil, eng)
	r.DELETE("/projects/:projectId/home/terminals/:sessionId", h.KillTerminal)

	rec := doReq(r, http.MethodDelete, "/projects/proj-x/home/terminals/sess1", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	eng.AssertNotCalled(t, "ListSessionsForWorkspace", mock.Anything)
}

func TestKillTerminal_Returns202(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("ListSessionsForWorkspace", "ws-1").Return([]string{"sess1"})
	eng.On("Kill", mock.Anything, "sess1").Return(nil)

	h := handlers.New(reader, nil, nil, eng)
	r.DELETE("/projects/:projectId/home/terminals/:sessionId", h.KillTerminal)

	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/terminals/sess1", nil)
	require.Equal(t, http.StatusAccepted, rec.Code)
	eng.AssertExpectations(t)
}

func TestKillTerminal_SessionNotInWorkspace_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("ListSessionsForWorkspace", "ws-1").Return([]string{"other-sess"})

	h := handlers.New(reader, nil, nil, eng)
	r.DELETE("/projects/:projectId/home/terminals/:sessionId", h.KillTerminal)

	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/terminals/ghost", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	eng.AssertNotCalled(t, "Kill", mock.Anything, mock.Anything)
}

func TestKillTerminal_KillReturnsErrSessionNotFound_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("ListSessionsForWorkspace", "ws-1").Return([]string{"sess1"})
	eng.On("Kill", mock.Anything, "sess1").Return(engineterminal.ErrSessionNotFound)

	h := handlers.New(reader, nil, nil, eng)
	r.DELETE("/projects/:projectId/home/terminals/:sessionId", h.KillTerminal)

	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/terminals/sess1", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestKillTerminal_KillGenericError_Returns500(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	reader := homeTerminalWorkspaceReader(t, "proj-1", "ws-1")
	eng := &mockTerminalEngine{}
	eng.On("ListSessionsForWorkspace", "ws-1").Return([]string{"sess1"})
	eng.On("Kill", mock.Anything, "sess1").Return(errors.New("pty gone"))

	h := handlers.New(reader, nil, nil, eng)
	r.DELETE("/projects/:projectId/home/terminals/:sessionId", h.KillTerminal)

	rec := doReq(r, http.MethodDelete, "/projects/proj-1/home/terminals/sess1", nil)
	require.Equal(t, http.StatusInternalServerError, rec.Code)
}

// ── TerminalWS ─────────────────────────────────────────────────────────

func TestTerminalWS_UnknownSession_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	eng := &mockTerminalEngine{}
	eng.On("SessionExists", mock.Anything, "ghost").Return(false)

	h := handlers.New(nil, nil, nil, eng)
	r.GET("/projects/:projectId/home/terminals/:sessionId/ws", h.TerminalWS)

	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/terminals/ghost/ws", nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	eng.AssertExpectations(t)
	eng.AssertNotCalled(t, "Attach", mock.Anything, mock.Anything, mock.Anything)
}

func TestTerminalWS_NonUpgradeRequest_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	eng := &mockTerminalEngine{}
	eng.On("SessionExists", mock.Anything, "sess1").Return(true)

	h := handlers.New(nil, nil, nil, eng)
	r.GET("/projects/:projectId/home/terminals/:sessionId/ws", h.TerminalWS)

	// A plain (non-Upgrade) GET request fails the websocket handshake.
	rec := doReq(r, http.MethodGet, "/projects/proj-1/home/terminals/sess1/ws", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	eng.AssertNotCalled(t, "Attach", mock.Anything, mock.Anything, mock.Anything)
}

// TestTerminalWS_SuccessfulUpgrade_AttachesSession drives a real websocket
// handshake against an httptest server to exercise the happy path: the
// upgrade succeeds and the engine's Attach is invoked with the resolved
// session id and connection.
func TestTerminalWS_SuccessfulUpgrade_AttachesSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	eng := &mockTerminalEngine{}
	eng.On("SessionExists", mock.Anything, "sess1").Return(true)
	attached := make(chan struct{})
	eng.On("Attach", mock.Anything, "sess1", mock.Anything).
		Run(func(_ mock.Arguments) { close(attached) }).
		Return(nil)

	h := handlers.New(nil, nil, nil, eng)
	r.GET("/projects/:projectId/home/terminals/:sessionId/ws", h.TerminalWS)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	url := "ws" + srv.URL[len("http"):] + "/projects/proj-1/home/terminals/sess1/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(url, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	select {
	case <-attached:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Attach to be called")
	}
	eng.AssertExpectations(t)
}
