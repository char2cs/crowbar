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

func TestRegister_MountsAllRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	editor.Register(
		r.Group("/v0"),
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
	}
	mounted := make(map[string]bool)
	for _, route := range r.Routes() {
		mounted[route.Path] = true
	}
	for _, path := range want {
		assert.True(t, mounted[path], "route %s not mounted", path)
	}
}
