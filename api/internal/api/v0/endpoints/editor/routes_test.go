package editor_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

type stubLSP struct{}

func (stubLSP) Completion(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) Hover(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) Definition(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) ([]domlsp.Location, error) {
	return nil, nil
}

func (stubLSP) References(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) ([]domlsp.Location, error) {
	return nil, nil
}

func (stubLSP) Rename(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
	_ string,
) (domlsp.WorkspaceEdit, error) {
	return domlsp.WorkspaceEdit{}, nil
}

func (stubLSP) CodeAction(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Range,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) DocumentSymbol(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) DidOpen(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	_ string,
) error {
	return nil
}

func (stubLSP) DidChange(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
) error {
	return nil
}

func (stubLSP) DidClose(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) error {
	return nil
}

func (stubLSP) DiagnosticsSnapshot(
	_ string,
) []domlsp.Diagnostic {
	return nil
}

type stubGit struct{}

func (stubGit) Blame(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.BlameEntry, error) {
	return nil, nil
}

type stubWSReader struct{}

func (stubWSReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	return domain.Workspace{WorktreePath: "/repo"}, nil
}

// editorSurface is the method+relative-path set editor.Register mounts,
// written once and asserted against BOTH live prefixes. The relative half is
// deliberately prefix-free: a route that reached only one of the two mounts is
// the failure this shape makes impossible to miss.
func editorSurface() []struct {
	method string
	path   string
} {
	return []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/blame?path=main.go"},
		{http.MethodPost, "/lsp/completion"},
		{http.MethodPost, "/lsp/hover"},
		{http.MethodPost, "/lsp/definition"},
		{http.MethodPost, "/lsp/references"},
		{http.MethodPost, "/lsp/rename"},
		{http.MethodPost, "/lsp/codeAction"},
		{http.MethodPost, "/lsp/documentSymbol"},
		{http.MethodGet, "/lsp/diagnostics"},
		{http.MethodPost, "/lsp/didOpen"},
		{http.MethodPost, "/lsp/didChange"},
		{http.MethodPost, "/lsp/didClose"},
	}
}

// registerBothMounts wires editor.Register the way router.go does: the old
// workspace-scoped group and the flat chat-scoped one, on one engine.
func registerBothMounts(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	editor.Register(
		v0,
		v0.Group("/chats/:chatId"),
		stubLSP{},
		stubGit{},
		stubWSReader{},
		func(_ *gin.Context) {},
	)
	return r
}

// TestRegisterMountsChatScopedRoutes is the route half of this step: every
// editor route is reachable at the flat /v0/chats/:chatId prefix (spec §7.1).
func TestRegisterMountsChatScopedRoutes(t *testing.T) {
	r := registerBothMounts(t)

	for _, tc := range editorSurface() {
		path := "/v0/chats/chat1" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegisterKeepsWorkspaceScopedRoutes is the regression bar for the
// coexistence this step deliberately ships: the workspace-scoped surface is
// NOT retired here (spec §8 step 6 does that, once every group has moved), so
// every one of its routes must still answer exactly as before.
func TestRegisterKeepsWorkspaceScopedRoutes(t *testing.T) {
	r := registerBothMounts(t)

	for _, tc := range editorSurface() {
		path := "/v0/workspaces/ws1" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegister_MountsLSPWSRoute pins the co-located diagnostics WS upgrade
// route on both prefixes, separately from editorSurface's REST routes since it
// takes a pre-built handler rather than a Handlers method.
func TestRegister_MountsLSPWSRoute(t *testing.T) {
	r := registerBothMounts(t)

	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}
	assert.True(t, mounted["GET /v0/workspaces/:wsId/lsp/ws"], "old lsp/ws route not mounted")
	assert.True(t, mounted["GET /v0/chats/:chatId/lsp/ws"], "new lsp/ws route not mounted")
}

func TestRegister_MountsAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	editor.Register(
		r.Group("/v0"),
		r.Group("/v0/chats/:chatId"),
		stubLSP{},
		stubGit{},
		stubWSReader{},
		func(_ *gin.Context) {},
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/workspaces/ws1/blame?path=main.go", nil)
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	want := []string{
		"/v0/workspaces/:wsId/blame",
		"/v0/workspaces/:wsId/lsp/completion",
		"/v0/workspaces/:wsId/lsp/hover",
		"/v0/workspaces/:wsId/lsp/definition",
		"/v0/workspaces/:wsId/lsp/references",
		"/v0/workspaces/:wsId/lsp/rename",
		"/v0/workspaces/:wsId/lsp/codeAction",
		"/v0/workspaces/:wsId/lsp/documentSymbol",
		"/v0/workspaces/:wsId/lsp/diagnostics",
		"/v0/workspaces/:wsId/lsp/didOpen",
		"/v0/workspaces/:wsId/lsp/didChange",
		"/v0/workspaces/:wsId/lsp/didClose",
		"/v0/chats/:chatId/blame",
		"/v0/chats/:chatId/lsp/completion",
		"/v0/chats/:chatId/lsp/hover",
		"/v0/chats/:chatId/lsp/definition",
		"/v0/chats/:chatId/lsp/references",
		"/v0/chats/:chatId/lsp/rename",
		"/v0/chats/:chatId/lsp/codeAction",
		"/v0/chats/:chatId/lsp/documentSymbol",
		"/v0/chats/:chatId/lsp/diagnostics",
		"/v0/chats/:chatId/lsp/didOpen",
		"/v0/chats/:chatId/lsp/didChange",
		"/v0/chats/:chatId/lsp/didClose",
	}
	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Path] = true
	}
	for _, path := range want {
		assert.True(t, mounted[path], "route %s not mounted", path)
	}
}
