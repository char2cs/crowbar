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
	"github.com/char2cs/crowbar/api/internal/api/v0/reqscope"
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

var errBoom = errors.New("boom")

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

// mountEditorRoutes registers editor's REST surface under rg — the shared
// route table newRouter and newRouterNoWorkspace both mount, so the two never
// drift into testing different surfaces.
func mountEditorRoutes(
	rg *gin.RouterGroup,
	h *editorhandlers.Handlers,
) {
	rg.GET("/blame", h.Blame)
	rg.POST("/lsp/completion", h.Completion)
	rg.POST("/lsp/hover", h.Hover)
	rg.POST("/lsp/definition", h.Definition)
	rg.POST("/lsp/references", h.References)
	rg.POST("/lsp/rename", h.Rename)
	rg.POST("/lsp/codeAction", h.CodeAction)
	rg.POST("/lsp/documentSymbol", h.DocumentSymbol)
	rg.GET("/lsp/diagnostics", h.Diagnostics)
	rg.POST("/lsp/didOpen", h.DidOpen)
	rg.POST("/lsp/didChange", h.DidChange)
	rg.POST("/lsp/didClose", h.DidClose)
}

// newRouter wires editor's handlers onto the flat chat-scoped group the way
// router.go does, with a stand-in for chatScoped's resolveChatWorktree
// middleware that always resolves a fixed workspace — the happy-path fixture
// every test in this package but the wiring-bug guard below uses.
func newRouter(
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
) *gin.Engine {
	r := gin.New()
	h := editorhandlers.New(lsp, git)
	rg := r.Group("/v0/chats/:chatId")
	rg.Use(func(c *gin.Context) {
		reqscope.SetWorkspace(c, domain.Workspace{WorktreePath: "/repo"})
		c.Next()
	})
	mountEditorRoutes(rg, h)
	return r
}

// newRouterNoWorkspace wires the same surface WITHOUT resolveChatWorktree's
// stand-in ever running, the wiring-bug case Handlers.workspace guards
// against: a route mounted outside the chat group's middleware.
func newRouterNoWorkspace(
	lsp editorhandlers.LSPEngine,
	git editorhandlers.GitEngine,
) *gin.Engine {
	r := gin.New()
	h := editorhandlers.New(lsp, git)
	mountEditorRoutes(r.Group("/v0/chats/:chatId"), h)
	return r
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
