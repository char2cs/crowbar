package chat_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// newProviderServer spins a gin router with the agent routes mounted over a REAL
// agent usecase whose provider surface (ResolveProviders / ReplaceProviderPreferences)
// is fully live: a real sqlite preference store, a temp crowbar home (so the catalog
// is the embedded claude+codex), and a deterministic install probe. The aggregate /
// terminal seams are nil because the provider endpoints never touch them.
func newProviderServer(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prefs, err := storesqlite.New[domain.AgentProviderPreference, string](":memory:")
	require.NoError(t, err)
	home := t.TempDir()
	homeFn := func() (string, error) { return home, nil }
	probe := func(a engineagents.Agent) bool { return a.ID() == "codex" }

	uc := agentusecase.New(agentusecase.Deps{
		Agents:        engineagents.New(),
		ProviderPrefs: prefs,
		Home:          homeFn,
		Installed:     probe,
	})

	r := gin.New()
	wsScoped := r.Group("/v0/projects/:projectId/repos/:repoId/workspaces/:wsId")
	settingsRG := r.Group("/v0")
	chat.Register(wsScoped, settingsRG, uc, uc, uc, uc, uc,
		nil, nil, func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func decodeProviderList(
	t *testing.T,
	rec *httptest.ResponseRecorder,
) []dto.AgentProviderDTO {
	t.Helper()
	var env struct {
		Success bool                   `json:"success"`
		Data    []dto.AgentProviderDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Success)
	return env.Data
}

func putProviders(
	t *testing.T,
	r *gin.Engine,
	body map[string]any,
) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v0/settings/chat/providers", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)
	return rec
}

// TestAgentProviders_EnrichedAndPreferences is the black-box contract for the
// provider surface: the enriched GET returns connected+enabled in priority order,
// the global PUT rewrites the order and disabled flags and echoes the resolved
// list, and an unknown id is a 400.
func TestAgentProviders_EnrichedAndPreferences(t *testing.T) {
	r := newProviderServer(t)

	// GET returns the enriched catalog in default (id) order, all enabled.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/v0/projects/p1/repos/r1/workspaces/ws-1/chats/providers", http.NoBody)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	list := decodeProviderList(t, rec)
	require.Equal(t, []string{"claude", "codex"}, providerIDsFromDTO(list))
	assert.True(t, list[0].Enabled, "providers default to enabled")
	assert.True(t, list[1].Enabled)
	assert.False(t, list[0].Connected, "claude probe returns false")
	assert.True(t, list[1].Connected, "codex probe returns true")

	// PUT reorders codex-first and disables claude; the response is the resolved list.
	rec = putProviders(t, r, map[string]any{"providers": []map[string]any{
		{"id": "codex", "disabled": false},
		{"id": "claude", "disabled": true},
	}})
	require.Equal(t, http.StatusOK, rec.Code)

	resolved := decodeProviderList(t, rec)
	require.Equal(t, []string{"codex", "claude"}, providerIDsFromDTO(resolved))
	assert.True(t, resolved[0].Enabled, "codex enabled")
	assert.False(t, resolved[1].Enabled, "claude disabled")

	// The change persisted: a fresh GET reflects the new order + disabled flag.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/v0/projects/p1/repos/r1/workspaces/ws-1/chats/providers", http.NoBody)
	r.ServeHTTP(rec, req)
	afterGet := decodeProviderList(t, rec)
	require.Equal(t, []string{"codex", "claude"}, providerIDsFromDTO(afterGet))
	assert.False(t, afterGet[1].Enabled)

	// Unknown id => 400, and nothing is applied.
	rec = putProviders(t, r, map[string]any{"providers": []map[string]any{
		{"id": "nope", "disabled": false},
	}})
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// The MCP toggle's whole round trip, and its polarity in every direction: the DB
// stores mcpDisabled, the PUT body takes mcpDisabled, and the wire reports
// mcpEnabled. A single inversion anywhere in that chain reads as a working
// feature in one direction and switches the tool surface off for everyone in the
// other, which is exactly why the default case is asserted first.
func TestAgentProviders_MCPToggleRoundTrip(t *testing.T) {
	r := newProviderServer(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/v0/projects/p1/repos/r1/workspaces/ws-1/chats/providers", http.NoBody)
	r.ServeHTTP(rec, req)
	for _, p := range decodeProviderList(t, rec) {
		assert.True(t, p.MCPEnabled, "%s must default to having its tool surface on", p.ID)
	}

	// A body that says nothing about MCP — the shape every client sent before this
	// existed — must leave the tool surface ON rather than writing a false that
	// reads as off.
	rec = putProviders(t, r, map[string]any{"providers": []map[string]any{
		{"id": "claude", "disabled": false},
		{"id": "codex", "disabled": false},
	}})
	require.Equal(t, http.StatusOK, rec.Code)
	for _, p := range decodeProviderList(t, rec) {
		assert.True(t, p.MCPEnabled, "%s lost its tool surface to a body that never mentioned it", p.ID)
	}

	// Switching one off leaves the other alone, and leaves the PROVIDER enabled.
	rec = putProviders(t, r, map[string]any{"providers": []map[string]any{
		{"id": "claude", "disabled": false, "mcpDisabled": true},
		{"id": "codex", "disabled": false},
	}})
	require.Equal(t, http.StatusOK, rec.Code)
	resolved := decodeProviderList(t, rec)
	require.Equal(t, []string{"claude", "codex"}, providerIDsFromDTO(resolved))
	assert.False(t, resolved[0].MCPEnabled, "claude's tool surface must be off")
	assert.True(t, resolved[0].Enabled, "switching the tools off must not disable the provider")
	assert.True(t, resolved[1].MCPEnabled, "codex's tool surface must be untouched")

	// And it persisted, rather than only being echoed back off the request body.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet,
		"/v0/projects/p1/repos/r1/workspaces/ws-1/chats/providers", http.NoBody)
	r.ServeHTTP(rec, req)
	after := decodeProviderList(t, rec)
	assert.False(t, after[0].MCPEnabled)
	assert.True(t, after[1].MCPEnabled)
}

func providerIDsFromDTO(
	list []dto.AgentProviderDTO,
) []string {
	ids := make([]string, len(list))
	for i, p := range list {
		ids[i] = p.ID
	}
	return ids
}
