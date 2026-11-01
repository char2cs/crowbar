package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

type stubEngine struct{}

func (stubEngine) Create(
	_ context.Context,
	_ string,
	_ string,
	_ *domain.TerminalProfile,
) (string, error) {
	return "sess1", nil
}

func (stubEngine) Kill(
	_ context.Context,
	_ string,
) error {
	return nil
}

func (stubEngine) SessionExists(
	_ context.Context,
	_ string,
) bool {
	return true
}

func (stubEngine) Attach(
	_ context.Context,
	_ string,
	_ engineterminal.WSConn,
) error {
	return nil
}

func (stubEngine) ListSessionsForChat(
	_ string,
) []string {
	return []string{"sess1"}
}

// SetHostTheme is a no-op here: the host theme's behaviour is covered where it has an
// observable effect (a session's OSC 11 answer at birth, in the terminal engine's own
// tests) and at the endpoint, by the recording engine in hosttheme_test.go.
func (stubEngine) SetHostTheme(_, _ color.Color) {}

func (stubEngine) StateOf(_ string) (string, bool) {
	return "active", true
}

type stubProfiles struct{}

func (stubProfiles) FindAll(
	_ context.Context,
) ([]domain.TerminalProfile, error) {
	return []domain.TerminalProfile{{ID: "p1"}}, nil
}

func (stubProfiles) FindByKey(
	_ context.Context,
	id string,
) (*domain.TerminalProfile, error) {
	return &domain.TerminalProfile{ID: id}, nil
}

func (stubProfiles) Save(
	_ context.Context,
	_ domain.TerminalProfile,
) error {
	return nil
}

func (stubProfiles) Delete(
	_ context.Context,
	_ string,
) error {
	return nil
}

// notFoundEngine returns ErrSessionNotFound on Kill so the 404 path can be
// exercised; it embeds stubEngine for the rest of the surface.
type notFoundEngine struct {
	stubEngine
}

func (notFoundEngine) Kill(
	_ context.Context,
	_ string,
) error {
	return fmt.Errorf("terminal: kill ghost: %w", engineterminal.ErrSessionNotFound)
}

// chatPath is the flat chat-scoped terminals collection path.
const chatPath = "/v0/chats/chat1/terminals"

func newHandlers() *handlers.Handlers {
	return handlers.New(stubEngine{}, stubProfiles{})
}

// scopeWorkspace stands in for v0's resolveChatWorktree middleware: it stashes
// the workspace the chat resolved to, which is where the handlers now read the
// PTY's working directory from.
func scopeWorkspace(
	c *gin.Context,
) {
	reqscope.SetWorkspace(c, domain.Workspace{ID: "ws1", WorktreePath: "/tmp/ws1", Provisioning: domain.WorkspaceProvisioned})
	c.Next()
}

func mountSessions(
	r *gin.Engine,
	h *handlers.Handlers,
) {
	scoped := r.Group("/v0/chats/:chatId")
	scoped.Use(scopeWorkspace)
	scoped.GET("/terminals", h.ListSessions)
	scoped.POST("/terminals", h.CreateSession)
	scoped.DELETE("/terminals/:sessionId", h.KillSession)
	scoped.GET("/terminals/:sessionId/ws", h.WS)
}

// mountSessionsUnscoped mounts the same routes WITHOUT the worktree-resolving
// middleware, so the handlers' "no workspace on the request context" branch can
// be exercised.
func mountSessionsUnscoped(
	r *gin.Engine,
	h *handlers.Handlers,
) {
	scoped := r.Group("/v0/chats/:chatId")
	scoped.GET("/terminals", h.ListSessions)
	scoped.POST("/terminals", h.CreateSession)
	scoped.DELETE("/terminals/:sessionId", h.KillSession)
	scoped.GET("/terminals/:sessionId/ws", h.WS)
}

func newRouter() *gin.Engine {
	r := gin.New()
	h := newHandlers()
	mountSessions(r, h)
	rg := r.Group("/v0")
	rg.GET("/settings/terminal/profiles", h.ListProfiles)
	rg.GET("/settings/terminal/profiles/:id", h.GetProfile)
	rg.POST("/settings/terminal/profiles", h.CreateProfile)
	rg.PUT("/settings/terminal/profiles/:id", h.UpdateProfile)
	rg.DELETE("/settings/terminal/profiles/:id", h.DeleteProfile)
	return r
}

func do(
	r *gin.Engine,
	method, path string,
	body any,
) *httptest.ResponseRecorder {
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

func TestTerminalHandlers_HappyPath(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodPost, chatPath, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodDelete, chatPath+"/sess1", nil) // KillSession
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodGet, "/v0/settings/terminal/profiles", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodGet, "/v0/settings/terminal/profiles/p1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodPost, "/v0/settings/terminal/profiles",
		map[string]any{"name": "default"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodPut, "/v0/settings/terminal/profiles/p1",
		map[string]any{"name": "updated"})
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, http.MethodDelete, "/v0/settings/terminal/profiles/p1", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestCreateSession_201WithTheSessionID(
	t *testing.T,
) {
	r := gin.New()
	mountSessions(r, newHandlers())

	rec := do(r, http.MethodPost, chatPath, map[string]any{"profileId": "prof1"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	var env struct {
		Data struct {
			SessionID string `json:"sessionId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "sess1", env.Data.SessionID)
}

func TestKillSession_200(
	t *testing.T,
) {
	r := gin.New()
	mountSessions(r, newHandlers())

	rec := do(r, http.MethodDelete, chatPath+"/sess1", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestListSessions_ReturnsScopedList(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodGet, chatPath, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data []dto.TerminalSessionDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	assert.Equal(t, "sess1", env.Data[0].ID)
	assert.Equal(t, "chat1", env.Data[0].ChatID)
	assert.Equal(t, "active", env.Data[0].Status)
}

// TestCreateSession_500WhenWorktreeUnresolved is the descendant of the old
// "404 on unknown workspace" case. Resolving the chat is no longer this
// handler's job — resolveChatWorktree does it, and 404s a chat whose worktree
// cannot be found before the handler runs (covered in v0's middleware_test).
// What is left here is the same refusal to spawn without a workspace, now
// reported as the wiring bug it can only be: nothing is created.
func TestCreateSession_500WhenWorktreeUnresolved(
	t *testing.T,
) {
	r := gin.New()
	h := handlers.New(stubEngine{}, stubProfiles{})
	mountSessionsUnscoped(r, h)

	rec := do(r, http.MethodPost, chatPath, nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestKillSession_404OnUnknownSession(
	t *testing.T,
) {
	r := gin.New()
	h := handlers.New(notFoundEngine{}, stubProfiles{})
	mountSessions(r, h)

	rec := do(r, http.MethodDelete, chatPath+"/ghost", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code, "a wrapped not-found is still a 404")
	assert.JSONEq(t, `{"success":false,"code":"not_found","error":"terminal: session not found"}`, rec.Body.String())
}
