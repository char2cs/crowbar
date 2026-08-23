package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/color"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
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

func (stubEngine) ListSessionsForWorkspace(
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

// spyBroadcaster records every pushed lifecycle DTO so handler tests can assert
// the active/ended frames synchronously (no sleep).
type spyBroadcaster struct {
	pushed []dto.TerminalSessionDTO
}

func (s *spyBroadcaster) Push(
	d dto.TerminalSessionDTO,
) {
	s.pushed = append(s.pushed, d)
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

type stubReader struct{}

func (stubReader) Get(
	_ context.Context,
	id string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: id}, nil
}

// errReader fails every Get so CreateSession's 404 path can be exercised.
type errReader struct{}

func (errReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, errors.New("not found")
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
	return engineterminal.ErrSessionNotFound
}

// wsPath is the hierarchical workspace-scoped terminals collection path.
const wsPath = "/v0/projects/p1/repos/r1/workspaces/ws1/terminals"

func newHandlers(
	broadcast handlers.TerminalBroadcaster,
) *handlers.Handlers {
	return handlers.New(stubEngine{}, stubProfiles{}, stubReader{}, broadcast)
}

func mountSessions(
	r *gin.Engine,
	h *handlers.Handlers,
) {
	scoped := r.Group("/v0/projects/:projectId/repos/:repoId/workspaces/:wsId")
	scoped.GET("/terminals", h.ListSessions)
	scoped.POST("/terminals", h.CreateSession)
	scoped.DELETE("/terminals/:sessionId", h.KillSession)
}

func newRouter() *gin.Engine {
	r := gin.New()
	h := newHandlers(&spyBroadcaster{})
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

	rec := do(r, http.MethodPost, wsPath, nil)
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodDelete, wsPath+"/sess1", nil) // KillSession
	assert.Equal(t, http.StatusAccepted, rec.Code)

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

func TestCreateSession_201AndBroadcast(
	t *testing.T,
) {
	r := gin.New()
	spy := &spyBroadcaster{}
	mountSessions(r, newHandlers(spy))

	rec := do(r, http.MethodPost, wsPath, map[string]any{"profileId": "prof1"})
	assert.Equal(t, http.StatusCreated, rec.Code)

	var env struct {
		Data struct {
			SessionID string `json:"sessionId"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "sess1", env.Data.SessionID)

	require.Len(t, spy.pushed, 1)
	active := spy.pushed[0]
	assert.Equal(t, "sess1", active.ID)
	assert.Equal(t, "p1", active.ProjectID)
	assert.Equal(t, "r1", active.RepoID)
	assert.Equal(t, "ws1", active.WorkspaceID)
	assert.Equal(t, "prof1", active.ProfileID)
	assert.Equal(t, "active", active.Status)
	assert.Nil(t, active.EndedAt)
}

func TestKillSession_202AndEndedBroadcast(
	t *testing.T,
) {
	r := gin.New()
	spy := &spyBroadcaster{}
	mountSessions(r, newHandlers(spy))

	rec := do(r, http.MethodDelete, wsPath+"/sess1", nil)
	assert.Equal(t, http.StatusAccepted, rec.Code)

	require.Len(t, spy.pushed, 1)
	ended := spy.pushed[0]
	assert.Equal(t, "sess1", ended.ID)
	assert.Equal(t, "p1", ended.ProjectID)
	assert.Equal(t, "r1", ended.RepoID)
	assert.Equal(t, "ws1", ended.WorkspaceID)
	assert.Equal(t, "ended", ended.Status)
	require.NotNil(t, ended.EndedAt)
}

func TestListSessions_ReturnsScopedList(
	t *testing.T,
) {
	r := newRouter()

	rec := do(r, http.MethodGet, wsPath, nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var env struct {
		Data []dto.TerminalSessionDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	assert.Equal(t, "sess1", env.Data[0].ID)
	assert.Equal(t, "ws1", env.Data[0].WorkspaceID)
	assert.Equal(t, "active", env.Data[0].Status)
}

func TestCreateSession_404OnUnknownWorkspace(
	t *testing.T,
) {
	r := gin.New()
	h := handlers.New(stubEngine{}, stubProfiles{}, errReader{}, &spyBroadcaster{})
	mountSessions(r, h)

	rec := do(r, http.MethodPost, wsPath, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestKillSession_404OnUnknownSession(
	t *testing.T,
) {
	r := gin.New()
	spy := &spyBroadcaster{}
	h := handlers.New(notFoundEngine{}, stubProfiles{}, stubReader{}, spy)
	mountSessions(r, h)

	rec := do(r, http.MethodDelete, wsPath+"/ghost", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, spy.pushed)
}
