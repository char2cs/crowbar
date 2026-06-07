package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestCompletion_200Raw(t *testing.T) {
	lsp := &fakeLSP{completion: json.RawMessage(`{"items":[{"label":"Println"}]}`)}
	r := newRouter(lsp, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/completion", map[string]any{
		"path":     "main.go",
		"position": map[string]int{"line": 1, "character": 2},
	})
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	var data map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &data))
	items, _ := data["items"].([]any)
	require.Len(t, items, 1)
}

func TestCompletion_NoServer_NullData(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/completion", map[string]any{
		"path": "file.unknownext",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.True(t, env.Success)
	assert.Equal(t, "null", string(env.Data))
}

func TestHover_200(t *testing.T) {
	r := newRouter(&fakeLSP{hover: json.RawMessage(`{"contents":"doc"}`)}, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/hover", map[string]any{
		"path": "main.go",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, decode(t, rec).Success)
}

func TestDefinition_200Locations(t *testing.T) {
	lsp := &fakeLSP{definition: []domlsp.Location{{FilePath: "/repo/x.go"}}}
	r := newRouter(lsp, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/definition", map[string]any{
		"path": "main.go",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	var locs []domlsp.Location
	require.NoError(t, json.Unmarshal(env.Data, &locs))
	require.Len(t, locs, 1)
	assert.Equal(t, "/repo/x.go", locs[0].FilePath)
}

func TestReferences_200EmptyArray(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/references", map[string]any{
		"path": "main.go",
	})
	require.Equal(t, http.StatusOK, rec.Code)

	env := decode(t, rec)
	assert.Equal(t, "[]", string(env.Data))
}

func TestRename_200(t *testing.T) {
	lsp := &fakeLSP{rename: domlsp.WorkspaceEdit{
		Changes: map[string][]domlsp.TextEdit{"main.go": {{NewText: "Foo"}}},
	}}
	r := newRouter(lsp, &fakeGit{}, okWSReader())

	rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/rename", map[string]any{
		"path":    "main.go",
		"newName": "Foo",
	})
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, decode(t, rec).Success)
}

func TestCodeActionDocumentSymbol_200(t *testing.T) {
	lsp := &fakeLSP{
		codeAction:     json.RawMessage(`[]`),
		documentSymbol: json.RawMessage(`[]`),
	}
	r := newRouter(lsp, &fakeGit{}, okWSReader())

	for _, route := range []string{"codeAction", "documentSymbol"} {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, map[string]any{
			"path":  "main.go",
			"range": map[string]any{"start": map[string]int{}, "end": map[string]int{}},
		})
		require.Equal(t, http.StatusOK, rec.Code, "route %s", route)
		assert.True(t, decode(t, rec).Success, "route %s", route)
	}
}

// lspRequestRoutes is every request/response LSP route with a body that
// satisfies binding, used by the table-driven guard tests.
func lspRequestRoutes() map[string]map[string]any {
	return map[string]map[string]any{
		"completion":     {"path": "main.go"},
		"hover":          {"path": "main.go"},
		"definition":     {"path": "main.go"},
		"references":     {"path": "main.go"},
		"rename":         {"path": "main.go", "newName": "Foo"},
		"codeAction":     {"path": "main.go"},
		"documentSymbol": {"path": "main.go"},
	}
}

func TestLSPRequest_BadBody_400(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, okWSReader())

	for route := range lspRequestRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, map[string]any{})
		assert.Equal(t, http.StatusBadRequest, rec.Code, "route %s", route)
		assert.False(t, decode(t, rec).Success, "route %s", route)
	}
}

func TestLSPRequest_UnknownWorkspace_404(t *testing.T) {
	r := newRouter(&fakeLSP{}, &fakeGit{}, &fakeWSReader{err: errBoom})

	for route, body := range lspRequestRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ghost/lsp/"+route, body)
		assert.Equal(t, http.StatusNotFound, rec.Code, "route %s", route)
	}
}

func TestLSPRequest_EngineError_500(t *testing.T) {
	r := newRouter(&fakeLSP{err: errBoom}, &fakeGit{}, okWSReader())

	for route, body := range lspRequestRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, body)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, "route %s", route)
	}
}

func TestLSPRequest_NilEngine_503(t *testing.T) {
	r := newRouter(nil, &fakeGit{}, okWSReader())

	for route, body := range lspRequestRoutes() {
		rec := do(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, body)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code, "route %s", route)
		assert.Equal(t, "lsp engine not available", decode(t, rec).Error, "route %s", route)
	}
}
