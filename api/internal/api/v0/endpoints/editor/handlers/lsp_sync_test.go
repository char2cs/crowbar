package handlers_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentSync_200(t *testing.T) {
	lsp := &fakeLSP{}
	r := newRouter(lsp, &fakeGit{}, okWSReader())

	open := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didOpen", map[string]any{
		"path":       "main.go",
		"languageId": "go",
		"text":       "package main",
	})
	require.Equal(t, http.StatusOK, open.Code)
	assert.True(t, decode(t, open).Success)

	change := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didChange", map[string]any{
		"path": "main.go",
		"text": "package main\n",
	})
	require.Equal(t, http.StatusOK, change.Code)

	closed := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didClose", map[string]any{
		"path": "main.go",
	})
	require.Equal(t, http.StatusOK, closed.Code)

	assert.Equal(t, 1, lsp.didOpenCalls)
	assert.Equal(t, 1, lsp.didChangeCalls)
	assert.Equal(t, 1, lsp.didCloseCalls)
}

func TestDidOpen_BadBody_400(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didOpen", map[string]any{
		"path": "main.go",
	})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// lspSyncRoutes is every document-sync route with a binding-satisfying body.
func lspSyncRoutes() map[string]map[string]any {
	return map[string]map[string]any{
		"didOpen":   {"path": "main.go", "languageId": "go"},
		"didChange": {"path": "main.go"},
		"didClose":  {"path": "main.go"},
	}
}

func TestLSPSync_BadBody_400(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, okWSReader())

	for route := range lspSyncRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, map[string]any{})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "route %s", route)
	}
}

func TestLSPSync_UnknownWorkspace_404(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, &fakeWSReader{err: errNotFound})

	for route, body := range lspSyncRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ghost/lsp/"+route, body)
		assert.Equal(t, http.StatusNotFound, rec.Code, "route %s", route)
	}
}

func TestLSPSync_EngineError_500(t *testing.T) {
	r := newRouter(&fakeLSP{err: errBoom}, &fakeGit{}, okWSReader())

	for route, body := range lspSyncRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, body)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "route %s", route)
	}
}

func TestLSPSync_NilEngine_503(t *testing.T) {
	r := newRouter(nil, &fakeGit{}, okWSReader())

	for route, body := range lspSyncRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, body)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "route %s", route)
	}
}
