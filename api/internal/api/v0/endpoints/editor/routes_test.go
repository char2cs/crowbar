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
	_ json.RawMessage,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) SignatureHelp(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) CodeLens(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) CodeLensResolve(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ json.RawMessage,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) SemanticTokens(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) SemanticTokensRange(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Range,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) ExecuteCommand(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
	_ json.RawMessage,
) (domlsp.CommandResult, error) {
	return domlsp.CommandResult{}, nil
}

func (stubLSP) Formatting(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.FormattingOptions,
) (json.RawMessage, error) {
	return nil, nil
}

func (stubLSP) Status(
	_ string,
	_ string,
) domlsp.ServerStatus {
	return domlsp.ServerStatus{State: domlsp.ServerStopped}
}

func (stubLSP) Restart(
	_ context.Context,
	_ string,
	_ string,
) (domlsp.ServerStatus, error) {
	return domlsp.ServerStatus{State: domlsp.ServerStopped}, nil
}

func (stubLSP) DidSave(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) error {
	return nil
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
		{http.MethodPost, "/lsp/signatureHelp"},
		{http.MethodPost, "/lsp/codeLens"},
		{http.MethodPost, "/lsp/codeLensResolve"},
		{http.MethodPost, "/lsp/formatting"},
		{http.MethodGet, "/lsp/status?path=main.go"},
		{http.MethodPost, "/lsp/restart"},
		{http.MethodGet, "/lsp/diagnostics"},
		{http.MethodPost, "/lsp/didOpen"},
		{http.MethodPost, "/lsp/didChange"},
		{http.MethodPost, "/lsp/didSave"},
		{http.MethodPost, "/lsp/didClose"},
	}
}

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
	)
	return r
}

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
