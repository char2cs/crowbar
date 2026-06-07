package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

func TestProviderState_200_WithPR(t *testing.T) {
	prov := &fakeProvider{state: providertypes.ProviderState{
		Protected: true,
		PR: &providertypes.PRInfo{
			Number:       7,
			Status:       "open",
			URL:          "https://example.com/pr/7",
			Title:        "feat: thing",
			TargetBranch: "main",
		},
	}}
	r := newRouter(prov, okWSReader())

	rec := do(t, r, "/v0/workspaces/ws1/provider")
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	var state dto.ProviderStateDTO
	require.NoError(t, json.Unmarshal(env.Data, &state))
	assert.True(t, state.Protected)
	require.NotNil(t, state.PR)
	assert.Equal(t, 7, state.PR.Number)
	assert.Equal(t, "main", state.PR.TargetBranch)
}

func TestProviderState_200_NoPR(t *testing.T) {
	prov := &fakeProvider{state: providertypes.ProviderState{Protected: false}}
	r := newRouter(prov, okWSReader())

	rec := do(t, r, "/v0/workspaces/ws1/provider")
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	var state dto.ProviderStateDTO
	require.NoError(t, json.Unmarshal(env.Data, &state))
	assert.False(t, state.Protected)
	assert.Nil(t, state.PR)
}

func TestProviderState_WorkspaceNotFound_404(t *testing.T) {
	r := newRouter(&fakeProvider{}, &fakeWSReader{err: errBoom})

	rec := do(t, r, "/v0/workspaces/ghost/provider")
	require.Equal(t, http.StatusNotFound, rec.Code)

	env := decode(t, rec)
	assert.False(t, env.Success)
	assert.Equal(t, "workspace not found", env.Error)
}

func TestProviderState_PollError_500(t *testing.T) {
	r := newRouter(&fakeProvider{pollErr: errBoom}, okWSReader())

	rec := do(t, r, "/v0/workspaces/ws1/provider")
	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.False(t, decode(t, rec).Success)
}

func TestProviderState_NilEngine_503(t *testing.T) {
	r := newRouter(nil, okWSReader())

	rec := do(t, r, "/v0/workspaces/ws1/provider")
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)

	env := decode(t, rec)
	assert.False(t, env.Success)
	assert.Equal(t, "provider engine not available", env.Error)
}
