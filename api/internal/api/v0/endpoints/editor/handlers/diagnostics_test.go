package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestDiagnostics_200Snapshot(t *testing.T) {
	lsp := &fakeLSP{snapshot: []domlsp.Diagnostic{{Message: "boom", Severity: "error"}}}
	r := newRouter(lsp, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodGet, "/v0/workspaces/ws1/lsp/diagnostics", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	var evt domlsp.DiagnosticsEvent
	require.NoError(t, json.Unmarshal(env.Data, &evt))
	assert.Equal(t, "ws1", evt.WsID)
	require.Len(t, evt.Diagnostics, 1)
	assert.Equal(t, "boom", evt.Diagnostics[0].Message)
}

func TestDiagnostics_UnknownWorkspace_404(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, &fakeWSReader{err: errNotFound})

	rec := do(t, r, http.MethodGet, "/v0/workspaces/ghost/lsp/diagnostics", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDiagnostics_NilEngine_503(t *testing.T) {
	r := newRouter(nil, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodGet, "/v0/workspaces/ws1/lsp/diagnostics", nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
