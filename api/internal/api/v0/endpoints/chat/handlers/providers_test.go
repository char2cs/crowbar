package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

// TestProviders_Success proves the GET handler forwards the usecase's resolved,
// enriched provider list (connected + enabled, in priority order) into the wire
// envelope unchanged.
func TestProviders_Success(t *testing.T) {
	uc := &fakeAgentUsecase{resolveProviders: []domain.AgentProvider{
		{ID: "codex", DisplayName: "Codex", Icon: "<svg/>", Connected: true, Enabled: true, Hotswap: true, HasTerminal: true, TerminalStartHere: false},
		{ID: "claude", DisplayName: "Claude", Icon: "<svg/>", Connected: false, Enabled: false, TerminalStartHere: true},
	}}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws-1/chats/providers", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Providers(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Success bool                   `json:"success"`
		Data    []dto.AgentProviderDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.True(t, env.Success)
	require.Len(t, env.Data, 2)
	assert.Equal(t, "codex", env.Data[0].ID)
	assert.True(t, env.Data[0].Connected)
	assert.True(t, env.Data[0].Enabled)
	assert.True(t, env.Data[0].Hotswap)
	assert.True(t, env.Data[0].HasTerminal)
	assert.False(t, env.Data[0].TerminalStartHere)
	assert.False(t, env.Data[1].Connected)
	assert.False(t, env.Data[1].Enabled)
	assert.False(t, env.Data[1].Hotswap)
	assert.False(t, env.Data[1].HasTerminal)
	assert.True(t, env.Data[1].TerminalStartHere)
}

// TestProviders_UsecaseError proves a ResolveProviders failure surfaces as a
// mapped error response rather than a 200.
func TestProviders_UsecaseError(t *testing.T) {
	uc := &fakeAgentUsecase{resolveErr: assert.AnError}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodGet, "/v0/projects/p1/repos/r1/workspaces/ws-1/chats/providers", nil)
	ctx.Params = gin.Params{{Key: "wsId", Value: "ws-1"}}

	h.Providers(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// The provider settings read every descriptor's findings — rule, severity,
// YAML path and line, hint — straight off the validator's report.
func TestDescriptorReports_ServesEveryFinding(t *testing.T) {
	uc := &fakeAgentUsecase{descriptorReports: []descriptorcheck.Report{{
		ID: "codex", Source: "/home/me/.crowbar/descriptors/codex.yaml",
		Findings: []descriptorcheck.Finding{{
			Rule: "session.locate_glob", Severity: descriptorcheck.SeverityError,
			Path: "session.locate.glob[0]", Line: 12, Message: "no {id}", Hint: "add {id}",
		}},
	}}}
	h := newChatHandlers(uc)
	ctx, rec := newTestContext(t, http.MethodGet, "/v0/settings/chat/descriptors", nil)

	h.DescriptorReports(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Data []struct {
			ID       string `json:"id"`
			Source   string `json:"source"`
			Findings []struct {
				Rule     string `json:"rule"`
				Severity string `json:"severity"`
				Path     string `json:"path"`
				Line     int    `json:"line"`
				Hint     string `json:"hint"`
			} `json:"findings"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data, 1)
	require.Len(t, env.Data[0].Findings, 1)
	f := env.Data[0].Findings[0]
	assert.Equal(t, "session.locate_glob", f.Rule)
	assert.Equal(t, "error", f.Severity)
	assert.Equal(t, "session.locate.glob[0]", f.Path)
	assert.Equal(t, 12, f.Line)
	assert.Equal(t, "add {id}", f.Hint)
}

func TestDescriptorReports_UsecaseError(t *testing.T) {
	h := newChatHandlers(&fakeAgentUsecase{descriptorErr: assert.AnError})
	ctx, rec := newTestContext(t, http.MethodGet, "/v0/settings/chat/descriptors", nil)

	h.DescriptorReports(ctx)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestUpdateProviderPreferences_ForwardsOrderedPrefs proves the PUT handler binds
// {providers:[{id,disabled}]} into ordered AgentProviderPreference rows (array
// index → Priority) and echoes the resolved list the usecase returns.
func TestUpdateProviderPreferences_ForwardsOrderedPrefs(t *testing.T) {
	uc := &fakeAgentUsecase{replaceResult: []domain.AgentProvider{
		{ID: "codex", DisplayName: "Codex", Enabled: true},
		{ID: "claude", DisplayName: "Claude", Enabled: false},
	}}
	h := newChatHandlers(uc)

	body := []byte(`{"providers":[{"id":"codex","disabled":false},{"id":"claude","disabled":true}]}`)
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/providers", body)

	h.UpdateProviderPreferences(ctx)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, uc.replaceCalls, 1)
	assert.Equal(t, []domain.AgentProviderPreference{
		{ProviderID: "codex", Priority: 0, Disabled: false},
		{ProviderID: "claude", Priority: 1, Disabled: true},
	}, uc.replaceCalls[0])

	var env struct {
		Success bool                   `json:"success"`
		Data    []dto.AgentProviderDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.Len(t, env.Data, 2)
	assert.Equal(t, "codex", env.Data[0].ID)
}

// TestUpdateProviderPreferences_BadJSON proves a malformed body is rejected 400
// without reaching the usecase.
func TestUpdateProviderPreferences_BadJSON(t *testing.T) {
	uc := &fakeAgentUsecase{}
	h := newChatHandlers(uc)

	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/providers", []byte("{not json"))

	h.UpdateProviderPreferences(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, uc.replaceCalls)
}

// TestUpdateProviderPreferences_UnknownProvider_MapsTo400 proves an
// apperr.ErrInvalidArgument from the usecase (an unknown provider id) surfaces as
// a 400.
func TestUpdateProviderPreferences_UnknownProvider_MapsTo400(t *testing.T) {
	uc := &fakeAgentUsecase{replaceErr: apperr.ErrInvalidArgument}
	h := newChatHandlers(uc)

	body := []byte(`{"providers":[{"id":"nope","disabled":false}]}`)
	ctx, rec := newTestContext(t, http.MethodPut, "/v0/settings/chat/providers", body)

	h.UpdateProviderPreferences(ctx)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
