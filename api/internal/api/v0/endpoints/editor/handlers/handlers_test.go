package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	editorhandlers "github.com/char2cs/crowbar/api/internal/api/v0/endpoints/editor/handlers"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	domlsp "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestMain(
	m *testing.M,
) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

var (
	errBoom     = errors.New("boom")
	errNotFound = apperr.ErrNotFound
)

type fakeLSP struct {
	completion     json.RawMessage
	hover          json.RawMessage
	codeAction     json.RawMessage
	documentSymbol json.RawMessage
	definition     []domlsp.Location
	references     []domlsp.Location
	rename         domlsp.WorkspaceEdit
	snapshot       []domlsp.Diagnostic
	err            error
	didOpenCalls   int
	didChangeCalls int
	didCloseCalls  int
}

func (f *fakeLSP) Completion(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	return f.completion, f.err
}

func (f *fakeLSP) Hover(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) (json.RawMessage, error) {
	return f.hover, f.err
}

func (f *fakeLSP) Definition(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) ([]domlsp.Location, error) {
	return f.definition, f.err
}

func (f *fakeLSP) References(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
) ([]domlsp.Location, error) {
	return f.references, f.err
}

func (f *fakeLSP) Rename(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Position,
	_ string,
) (domlsp.WorkspaceEdit, error) {
	return f.rename, f.err
}

func (f *fakeLSP) CodeAction(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ domlsp.Range,
) (json.RawMessage, error) {
	return f.codeAction, f.err
}

func (f *fakeLSP) DocumentSymbol(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) (json.RawMessage, error) {
	return f.documentSymbol, f.err
}

func (f *fakeLSP) DidOpen(
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

func (f *fakeLSP) DidChange(
	_ context.Context,
	_ string,
	_ string,
	_ string,
	_ string,
) error {
	f.didChangeCalls++
	return f.err
}

func (f *fakeLSP) DidClose(
	_ context.Context,
	_ string,
	_ string,
	_ string,
) error {
	f.didCloseCalls++
	return f.err
}

func (f *fakeLSP) DiagnosticsSnapshot(
	_ string,
) []domlsp.Diagnostic {
	return f.snapshot
}

var _ editorhandlers.LSPEngine = (*fakeLSP)(nil)

type fakeGit struct {
	entries []gitdomain.BlameEntry
	err     error
}

func (f *fakeGit) Blame(
	_ context.Context,
	_ string,
	_ string,
) ([]gitdomain.BlameEntry, error) {
	return f.entries, f.err
}

var _ editorhandlers.GitEngine = (*fakeGit)(nil)

type fakeWSReader struct {
	worktreePath string
	err          error
}

func (f *fakeWSReader) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	if f.err != nil {
		return domain.Workspace{}, f.err
	}
	return domain.Workspace{WorktreePath: f.worktreePath}, nil
}

var _ editorhandlers.WorkspaceReader = (*fakeWSReader)(nil)

func newRouter(
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
	wsReader editorhandlers.WorkspaceReader,
) *gin.Engine {
	r := gin.New()
	h := editorhandlers.New(lsp, git, wsReader)
	rg := r.Group("/v0")
	rg.GET("/workspaces/:wsId/blame", h.Blame)
	rg.POST("/workspaces/:wsId/lsp/completion", h.Completion)
	rg.POST("/workspaces/:wsId/lsp/hover", h.Hover)
	rg.POST("/workspaces/:wsId/lsp/definition", h.Definition)
	rg.POST("/workspaces/:wsId/lsp/references", h.References)
	rg.POST("/workspaces/:wsId/lsp/rename", h.Rename)
	rg.POST("/workspaces/:wsId/lsp/codeAction", h.CodeAction)
	rg.POST("/workspaces/:wsId/lsp/documentSymbol", h.DocumentSymbol)
	rg.GET("/workspaces/:wsId/lsp/diagnostics", h.Diagnostics)
	rg.POST("/workspaces/:wsId/lsp/didOpen", h.DidOpen)
	rg.POST("/workspaces/:wsId/lsp/didChange", h.DidChange)
	rg.POST("/workspaces/:wsId/lsp/didClose", h.DidClose)
	return r
}

func okWSReader() *fakeWSReader {
	return &fakeWSReader{worktreePath: "/repo"}
}

func do(
	t *testing.T,
	r *gin.Engine,
	method string,
	target string,
	body any,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, target, nil)
	} else {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		req = httptest.NewRequest(method, target, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
	}
	r.ServeHTTP(rec, req)
	return rec
}

type envelope struct {
	Success bool            `json:"success"`
	Error   string          `json:"error"`
	Data    json.RawMessage `json:"data"`
}

func decode(
	t *testing.T,
	rec *httptest.ResponseRecorder,
) envelope {
	t.Helper()
	var env envelope
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env
}
