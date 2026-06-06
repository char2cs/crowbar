package v0_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
	enginelsp "github.com/char2cs/crowbar/api/internal/engine/lsp"
)

// fakeLSPEngine is a canned enginelsp.Engine for handler tests. A request for a
// path ending in ".go" yields a fixed completion; other paths return empty
// results to exercise graceful absence.
type fakeLSPEngine struct {
	completionByPath map[string]json.RawMessage
	snapshot         []domlsp.Diagnostic
	err              error
	didOpenCalls     int
	didChangeCalls   int
	didCloseCalls    int
}

func (f *fakeLSPEngine) Completion(
	_ context.Context,
	_ string,
	_ string,
	filePath string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.completionByPath[filePath], nil
}

func (f *fakeLSPEngine) Hover(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	return nil, f.err
}

func (f *fakeLSPEngine) Definition(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) ([]domlsp.Location, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []domlsp.Location{{FilePath: "/repo/x.go"}}, nil
}

func (f *fakeLSPEngine) References(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) ([]domlsp.Location, error) {
	return nil, f.err
}

func (f *fakeLSPEngine) Rename(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
	_ string,
) (domlsp.WorkspaceEdit, error) {
	return domlsp.WorkspaceEdit{}, f.err
}

func (f *fakeLSPEngine) CodeAction(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Range,
) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`[]`), nil
}

func (f *fakeLSPEngine) DocumentSymbol(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (json.RawMessage, error) {
	if f.err != nil {
		return nil, f.err
	}
	return json.RawMessage(`[]`), nil
}

func (f *fakeLSPEngine) DidOpen(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	_ string,
) error {
	f.didOpenCalls++
	return f.err
}

func (f *fakeLSPEngine) DidChange(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
) error {
	f.didChangeCalls++
	return f.err
}

func (f *fakeLSPEngine) DidClose(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) error {
	f.didCloseCalls++
	return f.err
}

func (f *fakeLSPEngine) OnDiagnostics(
	_ func(domlsp.DiagnosticsEvent),
) {
}

func (f *fakeLSPEngine) DiagnosticsSnapshot(
	_ string,
) []domlsp.Diagnostic {
	return f.snapshot
}

func (f *fakeLSPEngine) Release(
	_ context.Context,
	_ string,
	_ string,
) {
}

var _ enginelsp.Engine = (*fakeLSPEngine)(nil)

func seedLSPWorkspace(
	t *testing.T,
	tc testContainers,
) {
	t.Helper()
	_, err := tc.app.Repositories.Workspace.Create(context.Background(), workspace.CreateInput{
		ID:           "ws1",
		RepoID:       "repo1",
		ProjectID:    "proj1",
		Branch:       "main",
		WorktreePath: t.TempDir(),
	}, time.Now())
	require.NoError(t, err)
}

func lspRouter(
	t *testing.T,
	tc testContainers,
) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c := v0.New(tc.app, tc.eng)
	r := gin.New()
	c.Register(r.Group("/v0"))
	return r
}

func TestLSP_Completion_GoFile_200(t *testing.T) {
	tc := newApp(t)
	fake := &fakeLSPEngine{completionByPath: map[string]json.RawMessage{
		"main.go": json.RawMessage(`{"items":[{"label":"Println"}]}`),
	}}
	tc.eng.LSP = fake
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/completion", map[string]any{
		"path":     "main.go",
		"position": map[string]int{"line": 1, "character": 2},
	})
	require.Equal(t, http.StatusOK, w.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	items, _ := got["items"].([]any)
	require.Len(t, items, 1)
}

func TestLSP_Completion_UnknownWorkspace_404(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ghost/lsp/completion", map[string]any{
		"path": "main.go",
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLSP_Completion_BadBody_400(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	// Missing required "path".
	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/completion", map[string]any{
		"position": map[string]int{"line": 1, "character": 2},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLSP_Completion_NoServer_200Empty(t *testing.T) {
	tc := newApp(t)
	// No canned completion for the path → engine returns nil (graceful absence).
	tc.eng.LSP = &fakeLSPEngine{completionByPath: map[string]json.RawMessage{}}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/completion", map[string]any{
		"path": "file.unknownext",
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "null", w.Body.String())
}

func TestLSP_Definition_200Locations(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/definition", map[string]any{
		"path": "main.go",
	})
	require.Equal(t, http.StatusOK, w.Code)

	var locs []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &locs))
	require.Len(t, locs, 1)
}

func TestLSP_References_200EmptyArray(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/references", map[string]any{
		"path": "main.go",
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "[]", w.Body.String())
}

func TestLSP_Hover_CodeAction_DocumentSymbol_200(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	for _, route := range []string{"hover", "codeAction", "documentSymbol"} {
		w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, map[string]any{
			"path":     "main.go",
			"position": map[string]int{"line": 0, "character": 0},
			"range":    map[string]any{"start": map[string]int{}, "end": map[string]int{}},
		})
		assert.Equal(t, http.StatusOK, w.Code, "route %s", route)
	}
}

func TestLSP_Rename_200(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/rename", map[string]any{
		"path":    "main.go",
		"newName": "Foo",
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestLSP_Rename_BadBody_400(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	// Missing required "newName".
	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/rename", map[string]any{
		"path": "main.go",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestLSP_Diagnostics_200Snapshot(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{snapshot: []domlsp.Diagnostic{{Message: "boom", Severity: "error"}}}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodGet, "/v0/workspaces/ws1/lsp/diagnostics", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var got domlsp.DiagnosticsEvent
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "ws1", got.WsID)
	require.Len(t, got.Diagnostics, 1)
	assert.Equal(t, "boom", got.Diagnostics[0].Message)
}

func TestLSP_Diagnostics_UnknownWorkspace_404(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	r := lspRouter(t, tc)

	w := doJSON(t, r, http.MethodGet, "/v0/workspaces/ghost/lsp/diagnostics", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestLSP_DocumentSync_200(t *testing.T) {
	tc := newApp(t)
	fake := &fakeLSPEngine{}
	tc.eng.LSP = fake
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	open := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didOpen", map[string]any{
		"path":       "main.go",
		"languageId": "go",
		"text":       "package main",
	})
	require.Equal(t, http.StatusOK, open.Code)

	change := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didChange", map[string]any{
		"path": "main.go",
		"text": "package main\n",
	})
	require.Equal(t, http.StatusOK, change.Code)

	closed := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didClose", map[string]any{
		"path": "main.go",
	})
	require.Equal(t, http.StatusOK, closed.Code)

	assert.Equal(t, 1, fake.didOpenCalls)
	assert.Equal(t, 1, fake.didChangeCalls)
	assert.Equal(t, 1, fake.didCloseCalls)
}

func TestLSP_DidOpen_BadBody_400(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	// Missing required "languageId".
	w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/didOpen", map[string]any{
		"path": "main.go",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// lspPostRoutes is every POST route with a valid body that satisfies binding,
// used by the table-driven 404 and 500 tests.
func lspPostRoutes() map[string]map[string]any {
	return map[string]map[string]any{
		"completion":     {"path": "main.go"},
		"hover":          {"path": "main.go"},
		"definition":     {"path": "main.go"},
		"references":     {"path": "main.go"},
		"rename":         {"path": "main.go", "newName": "Foo"},
		"codeAction":     {"path": "main.go"},
		"documentSymbol": {"path": "main.go"},
		"didOpen":        {"path": "main.go", "languageId": "go"},
		"didChange":      {"path": "main.go"},
		"didClose":       {"path": "main.go"},
	}
}

func TestLSP_AllPostRoutes_UnknownWorkspace_404(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	r := lspRouter(t, tc)

	for route, body := range lspPostRoutes() {
		w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ghost/lsp/"+route, body)
		assert.Equal(t, http.StatusNotFound, w.Code, "route %s", route)
	}
}

func TestLSP_AllPostRoutes_BadBody_400(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	for route := range lspPostRoutes() {
		// Empty body fails the required "path" (or "newName"/"languageId") binding.
		w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, map[string]any{})
		assert.Equal(t, http.StatusBadRequest, w.Code, "route %s", route)
	}
}

func TestLSP_AllPostRoutes_EngineError_500(t *testing.T) {
	tc := newApp(t)
	tc.eng.LSP = &fakeLSPEngine{err: errLSPBoom}
	seedLSPWorkspace(t, tc)
	r := lspRouter(t, tc)

	for route, body := range lspPostRoutes() {
		w := doJSON(t, r, http.MethodPost, "/v0/workspaces/ws1/lsp/"+route, body)
		assert.Equal(t, http.StatusInternalServerError, w.Code, "route %s", route)
	}
}

var errLSPBoom = errors.New("lsp boom")
