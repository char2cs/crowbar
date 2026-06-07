package terminal_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal"
	terminalhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/terminal/handlers"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

type stubProfiles struct{}

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

func (stubProfiles) FindByKey(
	_ context.Context,
	_ string,
) (*domain.TerminalProfile, error) {
	return nil, nil
}

func (stubProfiles) FindAll(
	_ context.Context,
) ([]domain.TerminalProfile, error) {
	return nil, nil
}

type stubWorkspace struct{}

func (stubWorkspace) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{}, nil
}

func TestRegister_MountsAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := engineterminal.New()
	t.Cleanup(eng.Shutdown)

	r := gin.New()
	terminal.Register(
		r.Group("/v0"),
		eng,
		stubProfiles{},
		stubWorkspace{},
		terminalhandlers.NewWS(eng),
	)

	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}
	assert.True(t, mounted["POST /v0/workspaces/:wsId/terminals"])
	assert.True(t, mounted["DELETE /v0/terminals/:sessionId"])
	assert.True(t, mounted["GET /v0/ws/terminals/:sessionId"])
	assert.True(t, mounted["GET /v0/settings/terminal/profiles"])
	assert.True(t, mounted["GET /v0/settings/terminal/profiles/:id"])
	assert.True(t, mounted["POST /v0/settings/terminal/profiles"])
	assert.True(t, mounted["PATCH /v0/settings/terminal/profiles/:id"])
	assert.True(t, mounted["DELETE /v0/settings/terminal/profiles/:id"])
}

func TestRegister_ListProfilesEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := engineterminal.New()
	t.Cleanup(eng.Shutdown)

	r := gin.New()
	terminal.Register(
		r.Group("/v0"),
		eng,
		stubProfiles{},
		stubWorkspace{},
		terminalhandlers.NewWS(eng),
	)

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/terminal/profiles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
	assert.Contains(t, w.Body.String(), `"data":[]`)
}
