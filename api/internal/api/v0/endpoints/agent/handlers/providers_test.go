package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/agent/handlers"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// TestProviders_Success proves Providers enumerates the usecase's descriptors
// into the {id, displayName, icon} wire shape and forwards the :wsId path
// param to ListProviders unchanged.
func TestProviders_Success(t *testing.T) {
	uc := &fakeAgentUsecase{providers: []engineagent.Descriptor{
		{ID: "claude", DisplayName: "Claude", Icon: "<svg/>"},
		{ID: "codex", DisplayName: "Codex", Icon: "<svg/>"},
	}}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws-1/agent/providers", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Providers(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Success bool `json:"success"`
		Data    []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Icon        string `json:"icon"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Success)
	require.Len(t, env.Data, 2)
	assert.Equal(t, "claude", env.Data[0].ID)
	assert.Equal(t, "Claude", env.Data[0].DisplayName)
	assert.Equal(t, "ws-1", uc.listProvidersWorkspace)
}

// TestProviders_UsecaseError proves a ListProviders failure surfaces as a
// mapped error response rather than a 200.
func TestProviders_UsecaseError(t *testing.T) {
	uc := &fakeAgentUsecase{providersErr: assert.AnError}
	h := handlers.New(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws-1/agent/providers", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Providers(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
