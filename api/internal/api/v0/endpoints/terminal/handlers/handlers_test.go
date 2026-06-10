package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
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

func newRouter() *gin.Engine {
	r := gin.New()
	h := handlers.New(stubEngine{}, stubProfiles{}, stubReader{})
	rg := r.Group("/v0")
	rg.POST("/workspaces/:wsId/terminals", h.CreateSession)
	rg.DELETE("/terminals/:sessionId", h.KillSession)
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

	rec := do(r, http.MethodPost, "/v0/workspaces/ws1/terminals", nil)
	assert.Equal(t, http.StatusCreated, rec.Code)

	rec = do(r, http.MethodDelete, "/v0/terminals/sess1", nil) // KillSession
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
