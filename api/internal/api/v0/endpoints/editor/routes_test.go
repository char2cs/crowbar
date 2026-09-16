package editor_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor"
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

// editorSurface is the method+relative-path set editor.Register mounts,
// written once. The relative half is deliberately prefix-free.
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

// registerChatScoped wires editor.Register the way router.go does: on the
// flat chat-scoped group alone (spec §8 step 6 retired the old
// workspace-scoped mount).
func registerChatScoped(
	t *testing.T,
) *gin.Engine {
	t.Helper()
	r := gin.New()
	v0 := r.Group("/v0")
	editor.Register(
		v0.Group("/chats/:chatId"),
		stubLSP{},
		stubGit{},
		func(_ *gin.Context) {},
	)
	return r
}

// TestRegisterMountsChatScopedRoutes is the route half of this step: every
// editor route is reachable at the flat /v0/chats/:chatId prefix (spec §7.1).
func TestRegisterMountsChatScopedRoutes(t *testing.T) {
	r := registerChatScoped(t)

	for _, tc := range editorSurface() {
		path := "/v0/chats/chat1" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.NotEqual(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegisterDropsWorkspaceScopedRoutes proves spec §8 step 6's deletion is
// real for editor/LSP: the old /v0/workspaces/:wsId/{blame,lsp/*} mount, kept
// alive alongside the chat-scoped one through the rest of this refactor,
// answers nothing any more.
func TestRegisterDropsWorkspaceScopedRoutes(t *testing.T) {
	r := registerChatScoped(t)

	for _, tc := range editorSurface() {
		path := "/v0/workspaces/ws1" + tc.path
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, path, http.NoBody)
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusNotFound, rec.Code, path)
	}
}

// TestRegister_MountsLSPWSRoute pins the co-located diagnostics WS upgrade
// route, separately from editorSurface's REST routes since it takes a
// pre-built handler rather than a Handlers method.
func TestRegister_MountsLSPWSRoute(t *testing.T) {
	r := registerChatScoped(t)

	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Method+" "+route.Path] = true
	}
	assert.True(t, mounted["GET /v0/chats/:chatId/lsp/ws"], "lsp/ws route not mounted")
	assert.False(t, mounted["GET /v0/workspaces/:wsId/lsp/ws"], "old lsp/ws route must be gone")
}
